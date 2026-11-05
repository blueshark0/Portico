package infrastructure

import "strings"

const (
	requestLogCostTierAll    = "all"
	requestLogCostTierLow    = "low"
	requestLogCostTierMedium = "medium"
	requestLogCostTierHigh   = "high"

	// 费用档位阈值（单位美元）：low < 0.1 <= medium <= 0.5 < high。
	// 前端着色只消费后端算好的 cost_tier 字段，阈值仅此一份。
	costTierLowMax    = 0.1
	costTierMediumMax = 0.5

	defaultRequestLogPageSize = 15
)

func normalizeRequestLogPagination(page int, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	switch pageSize {
	case 15, 30, 50, 100, 200:
	default:
		pageSize = defaultRequestLogPageSize
	}
	return page, pageSize
}

func normalizeRequestLogCostTier(costTier string) string {
	switch strings.ToLower(strings.TrimSpace(costTier)) {
	case requestLogCostTierLow:
		return requestLogCostTierLow
	case requestLogCostTierMedium:
		return requestLogCostTierMedium
	case requestLogCostTierHigh:
		return requestLogCostTierHigh
	default:
		return requestLogCostTierAll
	}
}

func classifyRequestLogCostTier(hasPricing bool, totalCost float64) string {
	if !hasPricing {
		return ""
	}
	switch {
	case totalCost < costTierLowMax:
		return requestLogCostTierLow
	case totalCost > costTierMediumMax:
		return requestLogCostTierHigh
	default:
		return requestLogCostTierMedium
	}
}

// costTierSQLClause 把费用档位过滤下推为落库 total_cost 列上的 SQL 条件，
// 返回追加到 WHERE 的子句与对应参数。tier 过滤依赖写入时物化的
// total_cost/has_pricing，不再把时间范围内全量日志载入内存逐行算费。
func costTierSQLClause(costTier string) (string, []any) {
	switch costTier {
	case requestLogCostTierLow:
		return " AND has_pricing = 1 AND total_cost < ?", []any{costTierLowMax}
	case requestLogCostTierMedium:
		return " AND has_pricing = 1 AND total_cost >= ? AND total_cost <= ?", []any{costTierLowMax, costTierMediumMax}
	case requestLogCostTierHigh:
		return " AND has_pricing = 1 AND total_cost > ?", []any{costTierMediumMax}
	default:
		return "", nil
	}
}
