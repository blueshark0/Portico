package infrastructure

import (
	"log"
	"sync"
	"time"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/storage"
	modelpricing "portico/resources/model-pricing"
)

var (
	sharedPricingOnce sync.Once
	sharedPricingSvc  *modelpricing.Service
)

// sharedPricingService 是查询侧与写入侧共用的定价服务：价格表只加载一次。
// NewServiceWithFile 对缺失/损坏的外部定价文件会告警并回退内置价，
// 因此除内置数据损坏外总是返回可用实例。
func sharedPricingService() *modelpricing.Service {
	sharedPricingOnce.Do(func() {
		pricingPath, _ := storage.AppDataPath("model_prices_and_context_window.json")
		svc, err := modelpricing.NewServiceWithFile(pricingPath)
		if err != nil {
			log.Printf("pricing service init failed: %v", err)
		}
		sharedPricingSvc = svc
	})
	return sharedPricingSvc
}

type SQLiteQueries struct {
	pricing *modelpricing.Service

	statsCache         *ttlCache[observabilitydomain.LogStats]
	providerDailyCache *ttlCache[[]observabilitydomain.ProviderDailyStat]
	heatmapCache       *ttlCache[[]observabilitydomain.HeatmapStat]
}

func NewSQLiteQueries() *SQLiteQueries {
	return &SQLiteQueries{
		pricing:            sharedPricingService(),
		statsCache:         newTTLCache(30*time.Second, cloneLogStats),
		providerDailyCache: newTTLCache(5*time.Minute, cloneProviderDailyStats),
		heatmapCache:       newTTLCache(5*time.Minute, cloneHeatmapStats),
	}
}

// decorateRequestLogCost 按当前定价表计算一条日志的费用明细与档位。
// 查询侧用它装配响应（明细与总额在响应内自洽）；写入侧用它得到
// 落库的 total_cost/has_pricing 物化值。
func decorateRequestLogCost(pricing *modelpricing.Service, logEntry *observabilitydomain.RequestLog) {
	if pricing == nil || logEntry == nil {
		return
	}
	usage := modelpricing.UsageSnapshot{
		InputTokens:       logEntry.InputTokens,
		OutputTokens:      logEntry.OutputTokens,
		CacheCreateTokens: logEntry.CacheCreateTokens,
		CacheReadTokens:   logEntry.CacheReadTokens,
		IsFast:            logEntry.IsFast,
	}
	cost := pricing.CalculateCost(logEntry.Model, usage)
	logEntry.HasPricing = cost.HasPricing
	logEntry.InputCost = cost.InputCost
	logEntry.OutputCost = cost.OutputCost
	logEntry.CacheCreateCost = cost.CacheCreateCost
	logEntry.CacheReadCost = cost.CacheReadCost
	logEntry.Ephemeral5mCost = cost.Ephemeral5mCost
	logEntry.Ephemeral1hCost = cost.Ephemeral1hCost
	logEntry.TotalCost = cost.TotalCost
	logEntry.CostTier = classifyRequestLogCostTier(cost.HasPricing, cost.TotalCost)
}

func (q *SQLiteQueries) decorateCost(logEntry *observabilitydomain.RequestLog) {
	if q == nil {
		return
	}
	decorateRequestLogCost(q.pricing, logEntry)
}

func (q *SQLiteQueries) calculateCost(model string, usage modelpricing.UsageSnapshot) modelpricing.CostBreakdown {
	if q == nil || q.pricing == nil {
		return modelpricing.CostBreakdown{}
	}
	return q.pricing.CalculateCost(model, usage)
}

func cloneLogStats(stats observabilitydomain.LogStats) observabilitydomain.LogStats {
	cloned := stats
	if stats.Series == nil {
		cloned.Series = []observabilitydomain.LogStatsSeries{}
		return cloned
	}
	cloned.Series = append([]observabilitydomain.LogStatsSeries(nil), stats.Series...)
	return cloned
}

func cloneProviderDailyStats(stats []observabilitydomain.ProviderDailyStat) []observabilitydomain.ProviderDailyStat {
	if stats == nil {
		return []observabilitydomain.ProviderDailyStat{}
	}
	return append([]observabilitydomain.ProviderDailyStat(nil), stats...)
}

func cloneHeatmapStats(stats []observabilitydomain.HeatmapStat) []observabilitydomain.HeatmapStat {
	if stats == nil {
		return []observabilitydomain.HeatmapStat{}
	}
	return append([]observabilitydomain.HeatmapStat(nil), stats...)
}
