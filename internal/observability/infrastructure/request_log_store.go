package infrastructure

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"portico/internal/shared/storage"
	modelpricing "portico/resources/model-pricing"
)

const RequestLogRetentionDays = 30

const requestLogCheckpointThreshold = 1000

var requestLogWriteMu sync.Mutex

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func requestLogCreatedAt(createdAt string) string {
	if strings.TrimSpace(createdAt) != "" {
		return createdAt
	}
	return time.Now().UTC().Format(timeLayout)
}

func CleanupOldRequestLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return err
	}

	requestLogWriteMu.Lock()
	defer requestLogWriteMu.Unlock()

	cutoff := fmt.Sprintf("-%d day", retentionDays)
	res, err := db.Exec("DELETE FROM request_log WHERE created_at < datetime('now', ?)", cutoff)
	if err != nil {
		if isNoSuchTableErr(err) {
			return nil
		}
		return err
	}
	if rows, err := res.RowsAffected(); err == nil && rows > 0 {
		log.Printf("清理 %d 条过期 request_log 记录\n", rows)
		if rows >= requestLogCheckpointThreshold {
			if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
				log.Printf("执行 wal_checkpoint 失败: %v\n", err)
			}
		}
	}
	if _, err := db.Exec("PRAGMA optimize"); err != nil {
		log.Printf("执行 PRAGMA optimize 失败: %v\n", err)
	}
	return nil
}

func isNoSuchTableErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}

func isSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_locked")
}

func EnsureRequestLogTableWithDB(db *sql.DB) error {
	const createTableSQL = `CREATE TABLE IF NOT EXISTS request_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT,
		model TEXT,
		provider TEXT,
		http_code INTEGER,
		input_tokens INTEGER,
		output_tokens INTEGER,
		cache_create_tokens INTEGER,
		cache_read_tokens INTEGER,
		reasoning_tokens INTEGER,
		is_stream INTEGER DEFAULT 0,
		is_fast INTEGER DEFAULT 0,
		duration_sec REAL DEFAULT 0,
		ttft_sec REAL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		total_cost REAL,
		has_pricing INTEGER,
		session_id TEXT
	)`

	if _, err := db.Exec(createTableSQL); err != nil {
		return err
	}

	// total_cost/has_pricing 不带 DEFAULT：NULL 表示"尚未计算"，由
	// backfillRequestLogCosts 回填；新写入总是带值。
	if err := storage.EnsureColumns(db, "request_log", []storage.ColumnDef{
		{Name: "created_at", Definition: "DATETIME"},
		{Name: "is_stream", Definition: "INTEGER DEFAULT 0"},
		{Name: "is_fast", Definition: "INTEGER DEFAULT 0"},
		{Name: "duration_sec", Definition: "REAL DEFAULT 0"},
		{Name: "ttft_sec", Definition: "REAL DEFAULT 0"},
		{Name: "total_cost", Definition: "REAL"},
		{Name: "has_pricing", Definition: "INTEGER"},
		{Name: "session_id", Definition: "TEXT"},
	}); err != nil {
		return err
	}
	if _, err := db.Exec("UPDATE request_log SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL OR created_at = ''"); err != nil {
		return err
	}
	if err := backfillRequestLogCosts(db, sharedPricingService()); err != nil {
		return err
	}

	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_request_log_platform_created_at ON request_log(platform, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_request_log_provider_created_at ON request_log(provider, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_request_log_platform_provider_created_at ON request_log(platform, provider, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_request_log_created_at ON request_log(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_request_log_platform_session ON request_log(platform, session_id)",
	}
	for _, stmt := range indexStatements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// backfillRequestLogCosts 为补列前写入的历史行（total_cost IS NULL）按当前
// 定价一次性计算并回填 total_cost/has_pricing，让费用档位过滤对旧数据
// 依然生效。之后的启动扫不到 NULL 行，本函数近似免费。
func backfillRequestLogCosts(db *sql.DB, pricing *modelpricing.Service) error {
	if pricing == nil {
		// 定价服务不可用时保留 NULL 标记，下次启动重试；
		// 这些行仅在费用档位过滤下暂时不可见。
		return nil
	}
	rows, err := db.Query(`SELECT id, model, input_tokens, output_tokens, cache_create_tokens, cache_read_tokens, is_fast
		FROM request_log WHERE total_cost IS NULL`)
	if err != nil {
		return err
	}

	type costUpdate struct {
		id         int64
		totalCost  float64
		hasPricing bool
	}
	updates := make([]costUpdate, 0)
	for rows.Next() {
		var id int64
		var model sql.NullString
		var input, output, cacheCreate, cacheRead, isFast sql.NullInt64
		if err := rows.Scan(&id, &model, &input, &output, &cacheCreate, &cacheRead, &isFast); err != nil {
			rows.Close()
			return err
		}
		cost := pricing.CalculateCost(model.String, modelpricing.UsageSnapshot{
			InputTokens:       int(input.Int64),
			OutputTokens:      int(output.Int64),
			CacheCreateTokens: int(cacheCreate.Int64),
			CacheReadTokens:   int(cacheRead.Int64),
			IsFast:            isFast.Int64 != 0,
		})
		updates = append(updates, costUpdate{id: id, totalCost: cost.TotalCost, hasPricing: cost.HasPricing})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(updates) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.Prepare("UPDATE request_log SET total_cost = ?, has_pricing = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, update := range updates {
		if _, err := stmt.Exec(update.totalCost, boolToInt(update.hasPricing), update.id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	log.Printf("已为 %d 条历史 request_log 记录回填费用列\n", len(updates))
	return nil
}
