package wails

import (
	observabilitydomain "portico/internal/observability/domain"
)

// ObservabilityQueries 是 facade 对请求日志查询的窄依赖（由
// observability/infrastructure.SQLiteQueries 满足）。读路径没有用例逻辑，
// facade 直接依赖查询接口，不再经过逐字转发的 application 层。
type ObservabilityQueries interface {
	ListRequestLogs(platform string, provider string, rangeKey string, costTier string, httpStatus string, page int, pageSize int) (observabilitydomain.RequestLogPage, error)
	ListSessionRequestLogs(platform string, sessionID string) ([]observabilitydomain.RequestLog, error)
	ListProviders(platform string) ([]string, error)
	StatsSince(platform string, provider string, rangeKey string, costTier string, httpStatus string) (observabilitydomain.LogStats, error)
	ProviderDailyStats(platform string) ([]observabilitydomain.ProviderDailyStat, error)
	HeatmapStats(days int) ([]observabilitydomain.HeatmapStat, error)
}

type ObservabilityFacade struct {
	queries ObservabilityQueries
}

func NewObservabilityFacade(queries ObservabilityQueries) *ObservabilityFacade {
	return &ObservabilityFacade{queries: queries}
}

func (f *ObservabilityFacade) ListRequestLogs(platform string, provider string, rangeKey string, costTier string, httpStatus string, page int, pageSize int) (observabilitydomain.RequestLogPage, error) {
	return f.queries.ListRequestLogs(platform, provider, rangeKey, costTier, httpStatus, page, pageSize)
}

func (f *ObservabilityFacade) ListSessionRequestLogs(platform string, sessionID string) ([]observabilitydomain.RequestLog, error) {
	return f.queries.ListSessionRequestLogs(platform, sessionID)
}

func (f *ObservabilityFacade) ListProviders(platform string) ([]string, error) {
	return f.queries.ListProviders(platform)
}

func (f *ObservabilityFacade) StatsSince(platform string, provider string, rangeKey string, costTier string, httpStatus string) (observabilitydomain.LogStats, error) {
	return f.queries.StatsSince(platform, provider, rangeKey, costTier, httpStatus)
}

func (f *ObservabilityFacade) ProviderDailyStats(platform string) ([]observabilitydomain.ProviderDailyStat, error) {
	return f.queries.ProviderDailyStats(platform)
}

func (f *ObservabilityFacade) HeatmapStats(days int) ([]observabilitydomain.HeatmapStat, error) {
	return f.queries.HeatmapStats(days)
}
