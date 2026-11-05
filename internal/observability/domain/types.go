package domain

type RequestLog struct {
	ID                int64   `json:"id"`
	Platform          string  `json:"platform"`
	Model             string  `json:"model"`
	Provider          string  `json:"provider"`
	SessionID         string  `json:"session_id"`
	HttpCode          int     `json:"http_code"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CacheCreateTokens int     `json:"cache_create_tokens"`
	CacheReadTokens   int     `json:"cache_read_tokens"`
	ReasoningTokens   int     `json:"reasoning_tokens"`
	IsStream          bool    `json:"is_stream"`
	IsFast            bool    `json:"is_fast"`
	DurationSec       float64 `json:"duration_sec"`
	// TTFTSec 是成功响应首条 data 事件或 JSON body 到达的耗时；
	// 0 表示历史记录或无可用数据，前端应显示为暂无数据。
	TTFTSec         float64 `json:"ttft_sec"`
	CreatedAt       string  `json:"created_at"`
	InputCost       float64 `json:"input_cost"`
	OutputCost      float64 `json:"output_cost"`
	CacheCreateCost float64 `json:"cache_create_cost"`
	CacheReadCost   float64 `json:"cache_read_cost"`
	Ephemeral5mCost float64 `json:"ephemeral_5m_cost"`
	Ephemeral1hCost float64 `json:"ephemeral_1h_cost"`
	TotalCost       float64 `json:"total_cost"`
	HasPricing      bool    `json:"has_pricing"`
	// CostTier 是后端统一计算的费用档位（""/low/medium/high），前端只消费
	// 该字段做着色，不再复写 0.1/0.5 阈值。
	CostTier string `json:"cost_tier"`
}

type RequestLogPage struct {
	Items    []RequestLog `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

// SessionUsage 是按 (platform, session_id) 聚合的用量摘要，供会话页把
// 不可读的会话 ID 换成"N 次请求 / tokens / 费用 / 用过的模型"这类可读信息。
// 只有成功转发后 relay 才会记录会话绑定，因此聚合只对已绑定会话有意义。
type SessionUsage struct {
	SessionID    string   `json:"session_id"`
	Requests     int      `json:"requests"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	TotalCost    float64  `json:"total_cost"`
	Models       []string `json:"models"`
}

type HeatmapStat struct {
	Day             string  `json:"day"`
	TotalRequests   int64   `json:"total_requests"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	TotalCost       float64 `json:"total_cost"`
}

type LogStats struct {
	TotalRequests     int64   `json:"total_requests"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	ReasoningTokens   int64   `json:"reasoning_tokens"`
	CacheCreateTokens int64   `json:"cache_create_tokens"`
	CacheReadTokens   int64   `json:"cache_read_tokens"`
	CostTotal         float64 `json:"cost_total"`
	CostInput         float64 `json:"cost_input"`
	CostOutput        float64 `json:"cost_output"`
	CostCacheCreate   float64 `json:"cost_cache_create"`
	CostCacheRead     float64 `json:"cost_cache_read"`
	// FailedRequests 与 AvgDurationSec 供指标卡使用；统计时忽略 httpStatus
	// 过滤（否则"只看成功"的默认视图下失败数恒为 0），其余过滤条件照常生效。
	FailedRequests   int64            `json:"failed_requests"`
	CacheHitRequests int64            `json:"cache_hit_requests"`
	AvgDurationSec   float64          `json:"avg_duration_sec"`
	Series           []LogStatsSeries `json:"series"`
}

type ProviderDailyStat struct {
	Provider           string  `json:"provider"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	SuccessRate        float64 `json:"success_rate"`
	InputTokens        int64   `json:"input_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
	ReasoningTokens    int64   `json:"reasoning_tokens"`
	CacheCreateTokens  int64   `json:"cache_create_tokens"`
	CacheReadTokens    int64   `json:"cache_read_tokens"`
	CostTotal          float64 `json:"cost_total"`
}

// AppLogEntry 是 ~/.portico/app.log 中的一条日志（续行已合并进 Message）。
type AppLogEntry struct {
	Time    string `json:"time"`    // "2026/07/08 18:27:22"，原样字符串；孤儿续行为 ""
	Source  string `json:"source"`  // "logging.go:23"（log.Lshortfile）
	Level   string `json:"level"`   // "INFO"/"WARN"/"ERROR"/"DEBUG"/""（无级别标签）
	Message string `json:"message"` // 已剥离级别标签，续行以 \n 拼接
}

type AppLogChunk struct {
	Entries    []AppLogEntry `json:"entries"`
	NextOffset int64         `json:"next_offset"` // 下次增量读取的起始字节偏移
	FileSize   int64         `json:"file_size"`
	Reset      bool          `json:"reset"`     // 文件轮转/截断，前端应清空已有列表
	Truncated  bool          `json:"truncated"` // 首次加载仅扫描了尾部窗口，更早日志未读取
}

type LogStatsSeries struct {
	Day               string  `json:"day"`
	TotalRequests     int64   `json:"total_requests"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	ReasoningTokens   int64   `json:"reasoning_tokens"`
	CacheCreateTokens int64   `json:"cache_create_tokens"`
	CacheReadTokens   int64   `json:"cache_read_tokens"`
	TotalCost         float64 `json:"total_cost"`
}
