package infrastructure

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portico/internal/shared/storage"
	modelpricing "portico/resources/model-pricing"
)

const RequestLogRetentionDays = 30

const requestLogCheckpointThreshold = 1000

var requestLogWriteMu sync.Mutex

// requestLogDataVersion 在 request_log 每次成功写入/删除后递增，供统计缓存
// 判断结果是否仍然有效（见 ttlCache 的 version）。
var requestLogDataVersion atomic.Uint64

func currentRequestLogDataVersion() uint64 {
	return requestLogDataVersion.Load()
}

func markRequestLogDataChanged() {
	requestLogDataVersion.Add(1)
}

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
		markRequestLogDataChanged()
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

// ensureRequestLogUnpricedIndex 让启动检查只与待回填行数成正比：建索引时全表扫描
// 一次，之后行回填完成就立刻离开这个部分索引。
func ensureRequestLogUnpricedIndex(db *sql.DB) error {
	requestLogWriteMu.Lock()
	defer requestLogWriteMu.Unlock()
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_request_log_unpriced ON request_log(id) WHERE total_cost IS NULL")
	return err
}

// RunRequestLogCostMaintenance 建立待回填索引并回填历史费用。首次升级时耗时与历史
// 数据量成正比，因此由启动后的后台维护调用，不阻塞窗口出现；中途退出下次启动继续。
func RunRequestLogCostMaintenance() error {
	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return err
	}
	if err := ensureRequestLogUnpricedIndex(db); err != nil {
		return err
	}
	return backfillRequestLogCosts(db, sharedPricingService())
}

// backfillRequestLogCosts 为补列前写入的历史行（total_cost IS NULL）按当前
// 定价一次性计算并回填 total_cost/has_pricing，让费用档位过滤对旧数据
// 依然生效。分批提交便于中断后重试；完成后部分索引为空。
func backfillRequestLogCosts(db *sql.DB, pricing *modelpricing.Service) error {
	if pricing == nil {
		// 定价服务不可用时保留 NULL 标记，下次启动重试；
		// 这些行仅在费用档位过滤下暂时不可见。
		return nil
	}
	var afterID int64
	total := 0
	for {
		updates, err := readRequestLogCostBatch(db, pricing, afterID)
		if err != nil {
			return err
		}
		if len(updates) == 0 {
			break
		}
		if err := writeRequestLogCostBatch(db, updates); err != nil {
			return err
		}
		afterID = updates[len(updates)-1].id
		total += len(updates)
	}
	if total > 0 {
		log.Printf("已为 %d 条历史 request_log 记录回填费用列\n", total)
	}
	return nil
}

const requestLogCostBatchSize = 500

type requestLogCostUpdate struct {
	id         int64
	totalCost  float64
	hasPricing bool
}

func readRequestLogCostBatch(db *sql.DB, pricing *modelpricing.Service, afterID int64) ([]requestLogCostUpdate, error) {
	rows, err := db.Query(`SELECT id, model, input_tokens, output_tokens, cache_create_tokens, cache_read_tokens, is_fast
		FROM request_log WHERE total_cost IS NULL AND id > ? ORDER BY id LIMIT ?`, afterID, requestLogCostBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	updates := make([]requestLogCostUpdate, 0, requestLogCostBatchSize)
	for rows.Next() {
		var id int64
		var model sql.NullString
		var input, output, cacheCreate, cacheRead, isFast sql.NullInt64
		if err := rows.Scan(&id, &model, &input, &output, &cacheCreate, &cacheRead, &isFast); err != nil {
			return nil, err
		}
		cost := pricing.CalculateCost(model.String, modelpricing.UsageSnapshot{
			InputTokens:       int(input.Int64),
			OutputTokens:      int(output.Int64),
			CacheCreateTokens: int(cacheCreate.Int64),
			CacheReadTokens:   int(cacheRead.Int64),
			IsFast:            isFast.Int64 != 0,
		})
		updates = append(updates, requestLogCostUpdate{id: id, totalCost: cost.TotalCost, hasPricing: cost.HasPricing})
	}
	return updates, rows.Err()
}

func writeRequestLogCostBatch(db *sql.DB, updates []requestLogCostUpdate) error {
	requestLogWriteMu.Lock()
	defer requestLogWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare("UPDATE request_log SET total_cost = ?, has_pricing = ? WHERE id = ? AND total_cost IS NULL")
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
	markRequestLogDataChanged()
	return nil
}
