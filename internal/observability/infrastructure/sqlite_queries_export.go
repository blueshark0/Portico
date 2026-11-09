package infrastructure

import observabilitydomain "portico/internal/observability/domain"

// requestLogExportMaxRows 是单次 CSV 导出的行数上限。
const requestLogExportMaxRows = 5000

// ExportRequestLogs 用一条查询取回全部导出行：不做 COUNT，也不用 OFFSET 翻页，
// 同一条语句读到的是同一快照，导出期间有新请求写入也不会出现重复行。
func (q *SQLiteQueries) ExportRequestLogs(platform string, provider string, rangeKey string, costTier string, httpStatus string) ([]observabilitydomain.RequestLog, error) {
	db, err := q.readDB()
	if err != nil {
		return nil, err
	}
	where, args := requestLogListWhere(platform, provider, rangeKey, costTier, httpStatus)
	args = append(args, requestLogExportMaxRows)
	items, err := q.queryRequestLogs(db, "SELECT "+requestLogSelectColumns+" FROM request_log"+where+" ORDER BY created_at DESC, id DESC LIMIT ?", args...)
	if isNoSuchTableErr(err) {
		return []observabilitydomain.RequestLog{}, nil
	}
	return items, err
}
