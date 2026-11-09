package infrastructure

import (
	"database/sql"
	"strings"

	observabilitydomain "portico/internal/observability/domain"
)

const requestLogSelectColumns = `
	id,
	COALESCE(platform, ''),
	COALESCE(model, ''),
	COALESCE(provider, ''),
	COALESCE(session_id, ''),
	COALESCE(http_code, 0),
	COALESCE(input_tokens, 0),
	COALESCE(output_tokens, 0),
	COALESCE(cache_create_tokens, 0),
	COALESCE(cache_read_tokens, 0),
	COALESCE(reasoning_tokens, 0),
	COALESCE(created_at, ''),
	COALESCE(is_stream, 0),
	COALESCE(is_fast, 0),
	COALESCE(duration_sec, 0),
	COALESCE(ttft_sec, 0)`

func (q *SQLiteQueries) ListRequestLogs(platform string, provider string, rangeKey string, costTier string, httpStatus string, page int, pageSize int) (observabilitydomain.RequestLogPage, error) {
	page, pageSize = normalizeRequestLogPagination(page, pageSize)
	result := observabilitydomain.RequestLogPage{
		Items:    []observabilitydomain.RequestLog{},
		Page:     page,
		PageSize: pageSize,
	}
	db, err := requestLogReadDB()
	if err != nil {
		return result, err
	}

	where, args := requestLogListWhere(platform, provider, rangeKey, costTier, httpStatus)
	if err := db.QueryRow("SELECT COUNT(*) FROM request_log"+where, args...).Scan(&result.Total); err != nil {
		if isNoSuchTableErr(err) {
			result.Total = 0
			return result, nil
		}
		return result, err
	}
	if result.Total == 0 {
		return result, nil
	}

	args = append(args, pageSize, (page-1)*pageSize)
	items, err := q.queryRequestLogs(db, requestLogListPageQuery(where), args...)
	if err != nil {
		if isNoSuchTableErr(err) {
			result.Total = 0
			return result, nil
		}
		return result, err
	}
	result.Items = items
	return result, nil
}

// requestLogListPageQuery 按 created_at 倒序分页：WHERE 以 created_at 范围
// 过滤，排序必须与之同列才能直接沿 idx_request_log_created_at 倒序取前 N 行；
// 若按 id 排序，SQLite 需要把范围内所有行取出再排序（7 天数万行时每页数百毫秒）。
// created_at 是请求结束入队的时刻，入队与落库同序，因此与旧的 id 倒序基本一致，
// 同一秒内再以 id 倒序打破平局，保证分页稳定。
func requestLogListPageQuery(where string) string {
	return "SELECT " + requestLogSelectColumns + " FROM request_log" + where +
		" ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
}

func requestLogListWhere(platform, provider, rangeKey, costTier, httpStatus string) (string, []any) {
	rangeSpec := buildLogRangeSpec(rangeKey, timeNow())
	where := " WHERE created_at >= ? AND created_at < ?"
	args := []any{rangeSpec.startUTCString(), rangeSpec.endUTCString()}
	if platform != "" {
		where += " AND platform = ?"
		args = append(args, platform)
	}
	if provider != "" {
		where += " AND provider = ?"
		args = append(args, provider)
	}
	costClause, costArgs := costTierSQLClause(normalizeRequestLogCostTier(costTier))
	where += costClause
	args = append(args, costArgs...)
	statusClause, statusArgs := httpStatusSQLClause(normalizeRequestLogHTTPStatus(httpStatus))
	where += statusClause
	args = append(args, statusArgs...)
	return where, args
}

func (q *SQLiteQueries) queryRequestLogs(db *sql.DB, query string, args ...any) ([]observabilitydomain.RequestLog, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]observabilitydomain.RequestLog, 0)
	for rows.Next() {
		var logEntry observabilitydomain.RequestLog
		if err := rows.Scan(
			&logEntry.ID,
			&logEntry.Platform,
			&logEntry.Model,
			&logEntry.Provider,
			&logEntry.SessionID,
			&logEntry.HttpCode,
			&logEntry.InputTokens,
			&logEntry.OutputTokens,
			&logEntry.CacheCreateTokens,
			&logEntry.CacheReadTokens,
			&logEntry.ReasoningTokens,
			&logEntry.CreatedAt,
			&logEntry.IsStream,
			&logEntry.IsFast,
			&logEntry.DurationSec,
			&logEntry.TTFTSec,
		); err != nil {
			return nil, err
		}
		q.decorateCost(&logEntry)
		logs = append(logs, logEntry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (q *SQLiteQueries) ListProviders(platform string) ([]string, error) {
	db, err := requestLogReadDB()
	if err != nil {
		return nil, err
	}
	query := "SELECT DISTINCT provider FROM request_log WHERE provider != ''"
	args := make([]any, 0, 1)
	if platform != "" {
		query += " AND platform = ?"
		args = append(args, platform)
	}
	query += " ORDER BY provider ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	providers := make([]string, 0)
	for rows.Next() {
		var provider string
		if err := rows.Scan(&provider); err != nil {
			return nil, err
		}
		if name := strings.TrimSpace(provider); name != "" {
			providers = append(providers, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return providers, nil
}
