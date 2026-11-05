package infrastructure

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/storage"
)

func (q *SQLiteQueries) HeatmapStats(days int) ([]observabilitydomain.HeatmapStat, error) {
	if days <= 0 {
		days = 30
	}
	cacheKey := fmt.Sprintf("days:%d", days)
	if cached, ok := q.heatmapCache.Get(cacheKey); ok {
		return cached, nil
	}
	totalHours := days * 24
	if totalHours <= 0 {
		totalHours = 24
	}

	rangeStart := startOfHour(time.Now())
	if totalHours > 1 {
		rangeStart = rangeStart.Add(-time.Duration(totalHours-1) * time.Hour)
	}

	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT strftime('%Y-%m-%d %H:00:00', created_at, 'localtime') AS bucket,
		model,
		COUNT(*) AS total_requests,
		SUM(input_tokens) AS input_tokens,
		SUM(output_tokens) AS output_tokens,
		SUM(reasoning_tokens) AS reasoning_tokens,
		SUM(cache_create_tokens) AS cache_create_tokens,
		SUM(cache_read_tokens) AS cache_read_tokens,
		is_fast
		FROM request_log
		WHERE created_at >= ?
		GROUP BY bucket, model, is_fast
		ORDER BY bucket DESC`, utcBoundString(rangeStart))
	if err != nil {
		if isNoSuchTableErr(err) {
			return []observabilitydomain.HeatmapStat{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	hourBuckets := map[int64]*observabilitydomain.HeatmapStat{}
	skipped := 0
	for rows.Next() {
		row, ok, err := scanUsageAggRow(rows, false)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		bucketTime, err := time.ParseInLocation(timeLayout, row.key, time.Local)
		if err != nil {
			skipped++
			continue
		}
		hourKey := bucketTime.Unix()
		bucket := hourBuckets[hourKey]
		if bucket == nil {
			bucket = &observabilitydomain.HeatmapStat{Day: bucketTime.Format("01-02 15")}
			hourBuckets[hourKey] = bucket
		}
		bucket.TotalRequests += row.totalRequests
		bucket.InputTokens += row.input
		bucket.OutputTokens += row.output
		bucket.ReasoningTokens += row.reasoning
		bucket.TotalCost += q.calculateCost(row.model, row.usageSnapshot()).TotalCost
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	warnSkippedStatRows("HeatmapStats", skipped)
	if len(hourBuckets) == 0 {
		return q.heatmapCache.Set(cacheKey, []observabilitydomain.HeatmapStat{}), nil
	}

	hourKeys := make([]int64, 0, len(hourBuckets))
	for key := range hourBuckets {
		hourKeys = append(hourKeys, key)
	}
	sort.Slice(hourKeys, func(i, j int) bool {
		return hourKeys[i] > hourKeys[j]
	})

	stats := make([]observabilitydomain.HeatmapStat, 0, min(len(hourKeys), totalHours))
	for _, key := range hourKeys {
		stats = append(stats, *hourBuckets[key])
		if len(stats) >= totalHours {
			break
		}
	}
	return q.heatmapCache.Set(cacheKey, stats), nil
}

func (q *SQLiteQueries) StatsSince(platform string, provider string, rangeKey string, costTier string, httpStatus string) (observabilitydomain.LogStats, error) {
	rangeSpec := buildLogRangeSpec(rangeKey, timeNow())
	normalizedCostTier := normalizeRequestLogCostTier(costTier)
	normalizedHTTPStatus := normalizeRequestLogHTTPStatus(httpStatus)
	cacheKey := fmt.Sprintf("platform:%s|provider:%s|range:%s|cost:%s|http:%s", platform, provider, rangeSpec.key, normalizedCostTier, normalizedHTTPStatus)
	if cached, ok := q.statsCache.Get(cacheKey); ok {
		return cached, nil
	}
	stats := observabilitydomain.LogStats{
		Series: make([]observabilitydomain.LogStatsSeries, 0, rangeSpec.seriesCount),
	}
	seriesBuckets, bucketIndexes := newStatsSeriesBuckets(rangeSpec)

	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return stats, err
	}

	args := []any{rangeSpec.startUTCString(), rangeSpec.endUTCString()}
	filterClause := ""
	if platform != "" {
		filterClause += " AND platform = ?"
		args = append(args, platform)
	}
	if provider != "" {
		filterClause += " AND provider = ?"
		args = append(args, provider)
	}
	// 费用档位过滤下推到落库的 total_cost 列，不再拉全量日志进内存逐行算费。
	tierClause, tierArgs := costTierSQLClause(normalizedCostTier)
	filterClause += tierClause
	args = append(args, tierArgs...)
	// 指标卡聚合沿用平台/供应商/档位过滤，但不套 httpStatus 过滤——
	// 否则"只看成功"的默认视图下失败数与延迟会失真。
	metricClause := filterClause
	metricArgs := append([]any(nil), args...)
	httpClause, httpArgs := httpStatusSQLClause(normalizedHTTPStatus)
	filterClause += httpClause
	args = append(args, httpArgs...)

	rows, err := db.Query(fmt.Sprintf(`SELECT strftime('%s', created_at, 'localtime') AS bucket,
			model,
			COUNT(*) AS total_requests,
			SUM(input_tokens) AS input_tokens,
		SUM(output_tokens) AS output_tokens,
			SUM(reasoning_tokens) AS reasoning_tokens,
			SUM(cache_create_tokens) AS cache_create_tokens,
			SUM(cache_read_tokens) AS cache_read_tokens,
			is_fast
			FROM request_log
			WHERE created_at >= ? AND created_at < ?%s
			GROUP BY bucket, model, is_fast
			ORDER BY bucket ASC`, rangeSpec.bucketSQLFormat(), filterClause), args...)
	if err != nil {
		if isNoSuchTableErr(err) {
			appendStatsSeries(&stats, seriesBuckets)
			return stats, nil
		}
		return stats, err
	}
	defer rows.Close()

	skipped := 0
	for rows.Next() {
		row, ok, err := scanUsageAggRow(rows, false)
		if err != nil {
			return stats, err
		}
		if !ok {
			continue
		}
		bucketTime, err := time.ParseInLocation(timeLayout, row.key, time.Local)
		if err != nil {
			skipped++
			continue
		}
		bucketIndex, ok := bucketIndexes[bucketTime.Format(timeLayout)]
		if !ok {
			continue
		}

		cost := q.calculateCost(row.model, row.usageSnapshot())
		bucket := seriesBuckets[bucketIndex]
		bucket.TotalRequests += row.totalRequests
		bucket.InputTokens += row.input
		bucket.OutputTokens += row.output
		bucket.ReasoningTokens += row.reasoning
		bucket.CacheCreateTokens += row.cacheCreate
		bucket.CacheReadTokens += row.cacheRead
		bucket.TotalCost += cost.TotalCost

		stats.TotalRequests += row.totalRequests
		stats.InputTokens += row.input
		stats.OutputTokens += row.output
		stats.ReasoningTokens += row.reasoning
		stats.CacheCreateTokens += row.cacheCreate
		stats.CacheReadTokens += row.cacheRead
		stats.CostInput += cost.InputCost
		stats.CostOutput += cost.OutputCost
		stats.CostCacheCreate += cost.CacheCreateCost
		stats.CostCacheRead += cost.CacheReadCost
		stats.CostTotal += cost.TotalCost
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	warnSkippedStatRows("StatsSince", skipped)

	var totalRequests, failedRequests, cacheHitRequests sql.NullInt64
	var avgDuration sql.NullFloat64
	metricRow := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) AS total_requests,
			SUM(CASE WHEN http_code >= 400 THEN 1 ELSE 0 END) AS failed_requests,
			SUM(CASE WHEN cache_read_tokens > 0 THEN 1 ELSE 0 END) AS cache_hit_requests,
			AVG(duration_sec) AS avg_duration_sec
			FROM request_log
			WHERE created_at >= ? AND created_at < ?%s`, metricClause), metricArgs...)
	if err := metricRow.Scan(&totalRequests, &failedRequests, &cacheHitRequests, &avgDuration); err != nil {
		// 表不存在视为空统计；其余错误如实上报。
		if !isNoSuchTableErr(err) {
			return stats, err
		}
	}
	stats.FailedRequests = nullInt64(failedRequests)
	stats.CacheHitRequests = nullInt64(cacheHitRequests)
	if avgDuration.Valid {
		stats.AvgDurationSec = avgDuration.Float64
	}

	appendStatsSeries(&stats, seriesBuckets)
	return q.statsCache.Set(cacheKey, stats), nil
}

// utcBoundString 把本地时间边界转成与 created_at 落库格式（UTC 字符串）可比较的形式。
// 所有统计查询的 WHERE 边界都必须经过它，避免再出现本地时间与 UTC 字符串直接比较的偏移。
func utcBoundString(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func newStatsSeriesBuckets(rangeSpec logRangeSpec) ([]*observabilitydomain.LogStatsSeries, map[string]int) {
	buckets := make([]*observabilitydomain.LogStatsSeries, 0, rangeSpec.seriesCount)
	indexes := make(map[string]int, rangeSpec.seriesCount)
	bucketTime := rangeSpec.startLocal

	for i := 0; i < rangeSpec.seriesCount; i++ {
		key := bucketTime.Format(timeLayout)
		buckets = append(buckets, &observabilitydomain.LogStatsSeries{Day: key})
		indexes[key] = i

		if rangeSpec.bucketGranularity == logRangeDaily {
			bucketTime = bucketTime.AddDate(0, 0, 1)
			continue
		}
		bucketTime = bucketTime.Add(time.Hour)
	}
	return buckets, indexes
}

func appendStatsSeries(stats *observabilitydomain.LogStats, seriesBuckets []*observabilitydomain.LogStatsSeries) {
	if stats == nil {
		return
	}
	for _, bucket := range seriesBuckets {
		if bucket == nil {
			continue
		}
		stats.Series = append(stats.Series, *bucket)
	}
}

func (q *SQLiteQueries) ProviderDailyStats(platform string) ([]observabilitydomain.ProviderDailyStat, error) {
	cacheKey := fmt.Sprintf("platform:%s", platform)
	if cached, ok := q.providerDailyCache.Get(cacheKey); ok {
		return cached, nil
	}
	start := startOfDay(time.Now())
	end := start.Add(24 * time.Hour)

	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return nil, err
	}

	args := []any{utcBoundString(start), utcBoundString(end)}
	platformFilter := ""
	if platform != "" {
		platformFilter = " AND platform = ?"
		args = append(args, platform)
	}

	rows, err := db.Query(fmt.Sprintf(`SELECT COALESCE(NULLIF(TRIM(provider), ''), '(unknown)') AS provider,
		model,
		COUNT(*) AS total_requests,
		SUM(CASE WHEN http_code BETWEEN 200 AND 299 THEN 1 ELSE 0 END) AS successful_requests,
		SUM(input_tokens) AS input_tokens,
		SUM(output_tokens) AS output_tokens,
		SUM(reasoning_tokens) AS reasoning_tokens,
		SUM(cache_create_tokens) AS cache_create_tokens,
		SUM(cache_read_tokens) AS cache_read_tokens,
		is_fast
		FROM request_log
		WHERE created_at >= ? AND created_at < ?%s
		GROUP BY provider, model, is_fast`, platformFilter), args...)
	if err != nil {
		if isNoSuchTableErr(err) {
			return []observabilitydomain.ProviderDailyStat{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	statMap := map[string]*observabilitydomain.ProviderDailyStat{}
	for rows.Next() {
		row, ok, err := scanUsageAggRow(rows, true)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		stat := statMap[row.key]
		if stat == nil {
			stat = &observabilitydomain.ProviderDailyStat{Provider: row.key}
			statMap[row.key] = stat
		}
		stat.TotalRequests += row.totalRequests
		stat.SuccessfulRequests += row.successfulReqs
		stat.FailedRequests += row.totalRequests - row.successfulReqs
		stat.InputTokens += row.input
		stat.OutputTokens += row.output
		stat.ReasoningTokens += row.reasoning
		stat.CacheCreateTokens += row.cacheCreate
		stat.CacheReadTokens += row.cacheRead
		stat.CostTotal += q.calculateCost(row.model, row.usageSnapshot()).TotalCost
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stats := make([]observabilitydomain.ProviderDailyStat, 0, len(statMap))
	for _, stat := range statMap {
		if stat.TotalRequests > 0 {
			stat.SuccessRate = float64(stat.SuccessfulRequests) / float64(stat.TotalRequests)
		}
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].TotalRequests == stats[j].TotalRequests {
			return stats[i].Provider < stats[j].Provider
		}
		return stats[i].TotalRequests > stats[j].TotalRequests
	})
	return q.providerDailyCache.Set(cacheKey, stats), nil
}
