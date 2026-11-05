package infrastructure

import (
	"strings"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/storage"
)

// ListSessionRequestLogs 返回本地保留期内一个会话的全部结构化请求记录，
// 供会话追踪视图切换请求发生当日与全周期；不读取或返回原始请求/响应体。
func (q *SQLiteQueries) ListSessionRequestLogs(platform string, sessionID string) ([]observabilitydomain.RequestLog, error) {
	platform = strings.TrimSpace(platform)
	sessionID = strings.TrimSpace(sessionID)
	if platform == "" || sessionID == "" {
		return []observabilitydomain.RequestLog{}, nil
	}
	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return nil, err
	}
	logs, err := q.queryRequestLogs(db,
		"SELECT "+requestLogSelectColumns+" FROM request_log WHERE platform = ? AND session_id = ? ORDER BY id DESC",
		platform,
		sessionID,
	)
	if err != nil {
		if isNoSuchTableErr(err) {
			return []observabilitydomain.RequestLog{}, nil
		}
		return nil, err
	}
	return logs, nil
}
