package infrastructure

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	sessiondomain "portico/internal/sessions/domain"
	"portico/internal/shared/kernel"
	"portico/internal/shared/storage"
)

type SQLiteRepository struct {
	dbName string
}

func NewSQLiteRepository(dbName string) *SQLiteRepository {
	if dbName == "" {
		dbName = storage.SessionDBName
	}
	return &SQLiteRepository{dbName: dbName}
}

func (r *SQLiteRepository) EnsureSchema() error {
	db, err := storage.DB(r.dbName)
	if err != nil {
		return err
	}
	return ensureSessionBindingTableWithDB(db)
}

func (r *SQLiteRepository) GetSessionProvider(platform, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	db, err := storage.DB(r.dbName)
	if err != nil {
		return "", fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var providerName string
	var lastSuccessAt time.Time
	query := `SELECT provider_name, last_success_at
		FROM session_provider_binding
		WHERE platform = ? AND session_id = ?`

	err = db.QueryRow(query, platform, sessionID).Scan(&providerName, &lastSuccessAt)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("查询会话绑定失败: %w", err)
	}

	if isExpired(platform, lastSuccessAt) {
		if err := deleteSessionBinding(db, platform, sessionID); err != nil {
			log.Printf("[WARN] 删除过期会话失败: %v\n", err)
		}
		return "", nil
	}

	return providerName, nil
}

func (r *SQLiteRepository) BindSessionToProvider(platform, sessionID, providerName string) error {
	if sessionID == "" || providerName == "" {
		return nil
	}
	db, err := storage.DB(r.dbName)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	now := time.Now()
	// Codex 新绑定先标记为未分类（2），请求成功后再由 RecordSessionBackground
	// 归为主会话（0）或纯后台任务（1），避免后台任务短暂出现在列表中。
	// 其他平台没有后台任务分类，沿用历史行为，直接保持可见。
	initialVisibility := 0
	if platform == kernel.PlatformCodex.String() {
		initialVisibility = 2
	}
	query := `INSERT INTO session_provider_binding (platform, session_id, provider_name, last_success_at, created_at, hidden)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, session_id)
		DO UPDATE SET provider_name = excluded.provider_name, last_success_at = excluded.last_success_at`

	if _, err := db.Exec(query, platform, sessionID, providerName, now, now, initialVisibility); err != nil {
		return fmt.Errorf("绑定会话失败: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) UpdateSessionSuccess(platform, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	db, err := storage.DB(r.dbName)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	query := `UPDATE session_provider_binding
		SET last_success_at = ?
		WHERE platform = ? AND session_id = ?`

	if _, err := db.Exec(query, time.Now(), platform, sessionID); err != nil {
		return fmt.Errorf("更新会话时间失败: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) RecordSessionBackground(platform, sessionID string, background bool) error {
	if sessionID == "" {
		return nil
	}
	db, err := storage.DB(r.dbName)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	backgroundValue := 0
	if background {
		backgroundValue = 1
	}
	// 主会话可见性具有优先性：记忆提取等后台请求可能复用主会话 ID，
	// 不能把已确认的主会话重新隐藏；隐藏会话出现主请求时则恢复可见。
	query := `UPDATE session_provider_binding
		SET hidden = CASE
			WHEN ? = 0 THEN 0
			WHEN hidden = 0 THEN 0
			ELSE 1
		END
		WHERE platform = ? AND session_id = ?`
	if _, err := db.Exec(query, backgroundValue, platform, sessionID); err != nil {
		return fmt.Errorf("更新会话可见性失败: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) CleanExpiredSessions() error {
	db, err := storage.DB(r.dbName)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var totalDeleted int64
	now := time.Now()
	for _, platform := range kernel.AllPlatforms() {
		cutoff := now.Add(-platform.SessionTimeout())
		result, err := db.Exec(
			`DELETE FROM session_provider_binding WHERE platform = ? AND last_success_at < ?`,
			platform.String(),
			cutoff,
		)
		if err != nil {
			return fmt.Errorf("清理过期会话失败: %w", err)
		}
		if rows, err := result.RowsAffected(); err == nil {
			totalDeleted += rows
		}
	}
	if totalDeleted > 0 {
		log.Printf("[INFO] 清理了 %d 个过期会话\n", totalDeleted)
	}
	return nil
}

func (r *SQLiteRepository) GetProviderSessions(platform, providerName string) ([]sessiondomain.SessionBinding, error) {
	if platform == "" || providerName == "" {
		return []sessiondomain.SessionBinding{}, nil
	}
	db, err := storage.DB(r.dbName)
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	query := `SELECT platform, session_id, provider_name, last_success_at, created_at
		FROM session_provider_binding
		WHERE platform = ? AND provider_name = ? AND hidden = 0
		ORDER BY last_success_at DESC`
	return scanActiveSessions(db, query, platform, providerName)
}

func (r *SQLiteRepository) GetPlatformSessions(platform string) ([]sessiondomain.SessionBinding, error) {
	if platform == "" {
		return []sessiondomain.SessionBinding{}, nil
	}
	db, err := storage.DB(r.dbName)
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	query := `SELECT platform, session_id, provider_name, last_success_at, created_at
		FROM session_provider_binding
		WHERE platform = ? AND hidden = 0
		ORDER BY provider_name ASC, last_success_at DESC`
	return scanActiveSessions(db, query, platform)
}

func (r *SQLiteRepository) UnbindSession(platform, sessionID string) error {
	db, err := storage.DB(r.dbName)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	if err := deleteSessionBinding(db, platform, sessionID); err != nil {
		return fmt.Errorf("解除会话绑定失败: %w", err)
	}
	return nil
}

func scanActiveSessions(db *sql.DB, query string, args ...any) ([]sessiondomain.SessionBinding, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询会话绑定失败: %w", err)
	}
	defer rows.Close()

	sessions := make([]sessiondomain.SessionBinding, 0)
	for rows.Next() {
		var session sessiondomain.SessionBinding
		if err := rows.Scan(
			&session.Platform,
			&session.SessionID,
			&session.ProviderName,
			&session.LastSuccessAt,
			&session.CreatedAt,
		); err != nil {
			log.Printf("[WARN] 扫描会话记录失败: %v\n", err)
			continue
		}
		if !isExpired(session.Platform, session.LastSuccessAt) {
			sessions = append(sessions, session)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历会话记录失败: %w", err)
	}
	return sessions, nil
}

func ensureSessionBindingTableWithDB(db *sql.DB) error {
	const createTableSQL = `CREATE TABLE IF NOT EXISTS session_provider_binding (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		session_id TEXT NOT NULL,
		provider_name TEXT NOT NULL,
		last_success_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		hidden INTEGER NOT NULL DEFAULT 0,
		UNIQUE(platform, session_id)
	)`
	if _, err := db.Exec(createTableSQL); err != nil {
		return err
	}
	if err := storage.EnsureColumns(db, "session_provider_binding", []storage.ColumnDef{
		{Name: "hidden", Definition: "INTEGER NOT NULL DEFAULT 0"},
	}); err != nil {
		return err
	}

	const createIndexSQL = `CREATE INDEX IF NOT EXISTS idx_session_lookup
		ON session_provider_binding(platform, session_id, last_success_at)`
	if _, err := db.Exec(createIndexSQL); err != nil {
		return err
	}

	extraIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_session_cleanup ON session_provider_binding(platform, last_success_at)",
		"CREATE INDEX IF NOT EXISTS idx_session_provider_lookup ON session_provider_binding(platform, provider_name, last_success_at DESC)",
	}
	for _, stmt := range extraIndexes {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func isExpired(platform string, lastSuccessAt time.Time) bool {
	timeout := kernel.Platform(platform).SessionTimeout()
	return time.Since(lastSuccessAt) > timeout
}

func deleteSessionBinding(db *sql.DB, platform, sessionID string) error {
	_, err := db.Exec(`DELETE FROM session_provider_binding WHERE platform = ? AND session_id = ?`, platform, sessionID)
	return err
}
