package infrastructure

import (
	"context"
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
	db, err := q.readDB()
	if err != nil {
		return result, err
	}
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	where, args := requestLogListWhere(platform, provider, rangeKey, costTier, httpStatus)
	if err := tx.QueryRow("SELECT COUNT(*) FROM request_log"+where, args...).Scan(&result.Total); err != nil {
		if isNoSuchTableErr(err) {
			result.Total = 0
			return result, nil
		}
		return result, err
	}
	if result.Total == 0 {
		return result, tx.Commit()
	}

	args = append(args, pageSize, (page-1)*pageSize)
	items, err := q.queryRequestLogs(tx, requestLogPageSQL(where), args...)
	if err != nil {
		if isNoSuchTableErr(err) {
			result.Total = 0
			return result, nil
		}
		return result, err
	}
	result.Items = items
	return result, tx.Commit()
}

// requestLogPageIDsSQL 按 created_at 倒序取当前页 ID：WHERE 以 created_at 范围
// 过滤，排序必须与之同列才能沿索引提前终止；若按 id 排序，SQLite 需要把范围内
// 所有行取出再排序（7 天十几万行时每页数百毫秒）。created_at 是请求结束入队的
// 时刻，与 id 顺序基本一致，同一秒内再以 id 倒序打破平局，保证分页稳定。
func requestLogPageIDsSQL(where string) string {
	return "SELECT id FROM request_log" + where + " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
}

// requestLogPageSQL 先物化当前页 ID，再按主键读取这些宽行，深分页时不必为跳过的行回表。
func requestLogPageSQL(where string) string {
	return "WITH page_ids AS MATERIALIZED (" + requestLogPageIDsSQL(where) + ") SELECT " + requestLogSelectColumns +
		" FROM request_log WHERE id IN (SELECT id FROM page_ids) ORDER BY created_at DESC, id DESC"
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

type requestLogQuerier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func (q *SQLiteQueries) queryRequestLogs(db requestLogQuerier, query string, args ...any) ([]observabilitydomain.RequestLog, error) {
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
	db, err := q.readDB()
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
