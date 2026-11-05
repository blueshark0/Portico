package infrastructure

import "time"

type LogRangeKey string

const (
	LogRangeToday     LogRangeKey = "today"
	LogRangeLast3Days LogRangeKey = "last3days"
	LogRangeLast7Days LogRangeKey = "last7days"
)

type logRangeGranularity int

const (
	logRangeHourly logRangeGranularity = iota
	logRangeDaily
)

type logRangeSpec struct {
	key               LogRangeKey
	startLocal        time.Time
	queryEndLocal     time.Time
	seriesEndLocal    time.Time
	seriesCount       int
	bucketGranularity logRangeGranularity
}

var timeNow = time.Now

func normalizeLogRangeKey(rangeKey string) LogRangeKey {
	switch LogRangeKey(rangeKey) {
	case LogRangeLast3Days:
		return LogRangeLast3Days
	case LogRangeLast7Days:
		return LogRangeLast7Days
	default:
		return LogRangeToday
	}
}

func buildLogRangeSpec(rangeKey string, now time.Time) logRangeSpec {
	nowLocal := now.In(time.Local)
	startToday := startOfDay(nowLocal)

	switch normalizeLogRangeKey(rangeKey) {
	case LogRangeLast3Days:
		startLocal := startToday.AddDate(0, 0, -2)
		return logRangeSpec{
			key:               LogRangeLast3Days,
			startLocal:        startLocal,
			queryEndLocal:     nowLocal,
			seriesEndLocal:    startLocal.AddDate(0, 0, 3),
			seriesCount:       3,
			bucketGranularity: logRangeDaily,
		}
	case LogRangeLast7Days:
		startLocal := startToday.AddDate(0, 0, -6)
		return logRangeSpec{
			key:               LogRangeLast7Days,
			startLocal:        startLocal,
			queryEndLocal:     nowLocal,
			seriesEndLocal:    startLocal.AddDate(0, 0, 7),
			seriesCount:       7,
			bucketGranularity: logRangeDaily,
		}
	default:
		return logRangeSpec{
			key:               LogRangeToday,
			startLocal:        startToday,
			queryEndLocal:     nowLocal,
			seriesEndLocal:    startToday.Add(24 * time.Hour),
			seriesCount:       24,
			bucketGranularity: logRangeHourly,
		}
	}
}

func (s logRangeSpec) startUTCString() string {
	return s.startLocal.UTC().Format(timeLayout)
}

func (s logRangeSpec) endUTCString() string {
	// request_log timestamps are stored at second precision, so move the
	// exclusive upper bound forward one second to include records created
	// during the current wall-clock second.
	return s.queryEndLocal.UTC().Add(time.Second).Format(timeLayout)
}

func (s logRangeSpec) bucketSQLFormat() string {
	if s.bucketGranularity == logRangeDaily {
		return "%Y-%m-%d 00:00:00"
	}
	return "%Y-%m-%d %H:00:00"
}
