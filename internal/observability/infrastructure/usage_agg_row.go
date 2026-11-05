package infrastructure

import (
	"database/sql"
	"log"

	modelpricing "portico/resources/model-pricing"
)

// usageAggRow 是三条聚合统计（HeatmapStats/StatsSince/ProviderDailyStats）
// 共用的行结构：新增 token 维度时只需改这里与各查询的 SELECT 列，
// 不再需要同步三份 Scan 与三份 UsageSnapshot 组装。
type usageAggRow struct {
	key            string // 时间 bucket 或 provider 名
	model          string
	totalRequests  int64
	successfulReqs int64 // 仅 withSuccess 查询填充
	input          int64
	output         int64
	reasoning      int64
	cacheCreate    int64
	cacheRead      int64
	isFast         bool
}

// scanUsageAggRow 按统一列序扫描一行：key, model, total_requests,
// [successful_requests,] input, output, reasoning, cache_create, cache_read, is_fast。
// 返回的 bool 表示该行 key 是否有效（无效行应跳过）。
func scanUsageAggRow(rows *sql.Rows, withSuccess bool) (usageAggRow, bool, error) {
	var key, model sql.NullString
	var totalRequests, successfulReqs sql.NullInt64
	var input, output, reasoning, cacheCreate, cacheRead, isFast sql.NullInt64

	dest := []any{&key, &model, &totalRequests}
	if withSuccess {
		dest = append(dest, &successfulReqs)
	}
	dest = append(dest, &input, &output, &reasoning, &cacheCreate, &cacheRead, &isFast)
	if err := rows.Scan(dest...); err != nil {
		return usageAggRow{}, false, err
	}
	if !key.Valid {
		return usageAggRow{}, false, nil
	}
	return usageAggRow{
		key:            key.String,
		model:          model.String,
		totalRequests:  nullInt64(totalRequests),
		successfulReqs: nullInt64(successfulReqs),
		input:          nullInt64(input),
		output:         nullInt64(output),
		reasoning:      nullInt64(reasoning),
		cacheCreate:    nullInt64(cacheCreate),
		cacheRead:      nullInt64(cacheRead),
		isFast:         nullInt64(isFast) != 0,
	}, true, nil
}

func (r usageAggRow) usageSnapshot() modelpricing.UsageSnapshot {
	return modelpricing.UsageSnapshot{
		InputTokens:       int(r.input),
		OutputTokens:      int(r.output),
		CacheCreateTokens: int(r.cacheCreate),
		CacheReadTokens:   int(r.cacheRead),
		IsFast:            r.isFast,
	}
}

// warnSkippedStatRows 汇总提示被丢弃的脏行：静默 continue 会让统计悄悄偏少。
func warnSkippedStatRows(queryName string, skipped int) {
	if skipped > 0 {
		log.Printf("[WARN] [observability] %s 丢弃了 %d 行无法解析时间的统计数据\n", queryName, skipped)
	}
}
