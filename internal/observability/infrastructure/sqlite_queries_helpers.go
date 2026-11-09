package infrastructure

import (
	"database/sql"
	"time"

	"portico/internal/shared/storage"
)

const timeLayout = "2006-01-02 15:04:05"

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func startOfHour(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, t.Hour(), 0, 0, 0, t.Location())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func nullInt64(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

// requestLogReadDB 返回 request_log 的查询连接池。只读池未注册时（测试、
// 内存库等单连接场景）回退到写入池，行为与旧实现一致。
func requestLogReadDB() (*sql.DB, error) {
	if db, err := storage.DB(storage.RequestLogReadDBName); err == nil {
		return db, nil
	}
	return storage.DB(storage.RequestLogDBName)
}
