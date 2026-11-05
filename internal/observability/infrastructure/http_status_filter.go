package infrastructure

import "strings"

const (
	requestLogHTTPStatusAll     = "all"
	requestLogHTTPStatusSuccess = "success"
	requestLogHTTPStatusFailed  = "failed"

	// 请求日志默认只展示 http_code = 200 的记录，需要排查失败请求时再切到 all。
	requestLogHTTPStatusSuccessCode = 200
)

// normalizeRequestLogHTTPStatus 归一化响应码过滤值；空串和非法值沿用默认 success。
func normalizeRequestLogHTTPStatus(httpStatus string) string {
	switch strings.ToLower(strings.TrimSpace(httpStatus)) {
	case requestLogHTTPStatusAll:
		return requestLogHTTPStatusAll
	case requestLogHTTPStatusFailed:
		return requestLogHTTPStatusFailed
	default:
		return requestLogHTTPStatusSuccess
	}
}

// httpStatusSQLClause 把响应码过滤下推为 http_code 列上的 SQL 条件，
// 返回追加到 WHERE 的子句与对应参数。
func httpStatusSQLClause(httpStatus string) (string, []any) {
	switch httpStatus {
	case requestLogHTTPStatusSuccess:
		return " AND http_code = ?", []any{requestLogHTTPStatusSuccessCode}
	case requestLogHTTPStatusFailed:
		return " AND http_code >= ?", []any{400}
	default:
		return "", nil
	}
}
