// Package modelpricing_test 黑盒锁定 model-pricing 包的现有计费行为。
//
// 重要说明：
//   - 所有模型名与价格均来自内置 model_prices_and_context_window.json 的真实条目，
//     以及 price.go 中硬编码的 ephemeral-1h / long-context 价格表。
//   - 仅使用 NewService / DefaultService / NewServiceWithFile(t.TempDir 内文件)，
//     不读取用户 HOME 或任何真实外部文件。
//   - 模糊匹配用例均为"单一候选"场景（已逐一核对内置表），避免 map 遍历序带来的
//     非确定性。long-context 兜底 for-range 遍历的 map 只有一个条目，同样确定。
package modelpricing_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	modelpricing "portico/resources/model-pricing"
)

// 内置价格表中的真实单价（美元/token）。
const (
	// claude-sonnet-4-5-20250929（内置 JSON 精确条目）
	sonnet45In   = 0.000003
	sonnet45Out  = 0.000015
	sonnet45C5m  = 0.00000375
	sonnet45Read = 0.0000003
	// claude-sonnet-4-5-20250929 不在硬编码 ephemeral-1h 表内，
	// 命中名称含 "sonnet" 的启发式分支。
	sonnet45C1h = 0.000006

	// claude-opus-4-1-20250805（内置 JSON 精确条目；也在硬编码 ephemeral-1h 表内）
	opus41In   = 0.000015
	opus41Out  = 0.000075
	opus41C5m  = 0.00001875
	opus41Read = 0.0000015
	opus41C1h  = 0.00003

	// claude-sonnet-4-20250514（内置 JSON 条目，[1m] 变体的基础价）
	sonnet4In   = 0.000003
	sonnet4Out  = 0.000015
	sonnet4C5m  = 0.00000375
	sonnet4Read = 0.0000003

	// buildLongContextPricing 中唯一条目 claude-sonnet-4-20250514[1m] 的长上下文单价
	longIn  = 0.000006
	longOut = 0.0000225

	// ai21.j2-mid-v1（内置 JSON 精确条目）：只有 input/output 单价，
	// 缓存单价由 ensureCachePricing 推导：create = input*1.25, read = input*0.1。
	j2In   = 0.0000125
	j2C5m  = j2In * 1.25
	j2Read = j2In * 0.1
)

func newService(t *testing.T) *modelpricing.Service {
	t.Helper()
	svc, err := modelpricing.NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned nil service")
	}
	return svc
}

func almostEqual(a, b float64) bool {
	diff := math.Abs(a - b)
	if diff <= 1e-15 {
		return true
	}
	return diff <= 1e-9*math.Max(math.Abs(a), math.Abs(b))
}

func assertBreakdown(t *testing.T, got, want modelpricing.CostBreakdown) {
	t.Helper()
	if got.HasPricing != want.HasPricing {
		t.Errorf("HasPricing = %v, want %v", got.HasPricing, want.HasPricing)
	}
	if got.IsLongContext != want.IsLongContext {
		t.Errorf("IsLongContext = %v, want %v", got.IsLongContext, want.IsLongContext)
	}
	fields := []struct {
		name      string
		got, want float64
	}{
		{"InputCost", got.InputCost, want.InputCost},
		{"OutputCost", got.OutputCost, want.OutputCost},
		{"CacheCreateCost", got.CacheCreateCost, want.CacheCreateCost},
		{"CacheReadCost", got.CacheReadCost, want.CacheReadCost},
		{"Ephemeral5mCost", got.Ephemeral5mCost, want.Ephemeral5mCost},
		{"Ephemeral1hCost", got.Ephemeral1hCost, want.Ephemeral1hCost},
		{"TotalCost", got.TotalCost, want.TotalCost},
	}
	for _, f := range fields {
		if !almostEqual(f.got, f.want) {
			t.Errorf("%s = %.12f, want %.12f", f.name, f.got, f.want)
		}
	}
}

func TestCalculateCost(t *testing.T) {
	svc := newService(t)

	tests := []struct {
		name  string
		model string
		usage modelpricing.UsageSnapshot
		want  modelpricing.CostBreakdown
	}{
		{
			name:  "普通 input/output 计价（精确命中）",
			model: "claude-sonnet-4-5-20250929",
			usage: modelpricing.UsageSnapshot{InputTokens: 1000, OutputTokens: 500},
			want: modelpricing.CostBreakdown{
				InputCost:  1000 * sonnet45In,
				OutputCost: 500 * sonnet45Out,
				TotalCost:  1000*sonnet45In + 500*sonnet45Out,
				HasPricing: true,
			},
		},
		{
			name:  "已知模型零用量：成本全 0 但 HasPricing=true",
			model: "claude-sonnet-4-5-20250929",
			usage: modelpricing.UsageSnapshot{},
			want:  modelpricing.CostBreakdown{HasPricing: true},
		},
		{
			name:  "未知模型：HasPricing=false 且成本全 0",
			model: "totally-unknown-model-xyz-987",
			usage: modelpricing.UsageSnapshot{InputTokens: 12345, OutputTokens: 678, CacheCreateTokens: 90, CacheReadTokens: 12},
			want:  modelpricing.CostBreakdown{},
		},
		{
			name:  "未知模型 + IsFast：仍然全 0",
			model: "totally-unknown-model-xyz-987",
			usage: modelpricing.UsageSnapshot{InputTokens: 1000, OutputTokens: 1000, IsFast: true},
			want:  modelpricing.CostBreakdown{},
		},
		{
			name:  "空模型名：返回零值",
			model: "",
			usage: modelpricing.UsageSnapshot{InputTokens: 1000, OutputTokens: 1000},
			want:  modelpricing.CostBreakdown{},
		},
		{
			name:  "cache 无明细：CacheCreateTokens 全按 5m 单价",
			model: "claude-opus-4-1-20250805",
			usage: modelpricing.UsageSnapshot{CacheCreateTokens: 1000},
			want: modelpricing.CostBreakdown{
				Ephemeral5mCost: 1000 * opus41C5m,
				CacheCreateCost: 1000 * opus41C5m,
				TotalCost:       1000 * opus41C5m,
				HasPricing:      true,
			},
		},
		{
			name:  "cache 明细恰好拆满：5m/1h 各按各自单价",
			model: "claude-opus-4-1-20250805",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 1000,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral5mTokens: 400, Ephemeral1hTokens: 600},
			},
			want: modelpricing.CostBreakdown{
				Ephemeral5mCost: 400 * opus41C5m,
				Ephemeral1hCost: 600 * opus41C1h,
				CacheCreateCost: 400*opus41C5m + 600*opus41C1h,
				TotalCost:       400*opus41C5m + 600*opus41C1h,
				HasPricing:      true,
			},
		},
		{
			name:  "cache 明细不足总量：差额补到 5m",
			model: "claude-opus-4-1-20250805",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 1000,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral5mTokens: 200, Ephemeral1hTokens: 300},
			},
			want: modelpricing.CostBreakdown{
				// remaining = 1000-200-300 = 500，加到 5m => 700
				Ephemeral5mCost: 700 * opus41C5m,
				Ephemeral1hCost: 300 * opus41C1h,
				CacheCreateCost: 700*opus41C5m + 300*opus41C1h,
				TotalCost:       700*opus41C5m + 300*opus41C1h,
				HasPricing:      true,
			},
		},
		{
			name:  "cache 明细超过总量：按明细计费（不截断）",
			model: "claude-opus-4-1-20250805",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 0,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral5mTokens: 100, Ephemeral1hTokens: 50},
			},
			want: modelpricing.CostBreakdown{
				// remaining = 0-100-50 = -150（不生效），仍按 (100, 50) 计费
				Ephemeral5mCost: 100 * opus41C5m,
				Ephemeral1hCost: 50 * opus41C1h,
				CacheCreateCost: 100*opus41C5m + 50*opus41C1h,
				TotalCost:       100*opus41C5m + 50*opus41C1h,
				HasPricing:      true,
			},
		},
		{
			name:  "cache 明细为负：差额回填后 5m=50、1h 钳位为 0",
			model: "claude-opus-4-1-20250805",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 0,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral5mTokens: -100, Ephemeral1hTokens: -50},
			},
			want: modelpricing.CostBreakdown{
				// remaining = 0-(-100)-(-50) = 150 > 0 => five = -100+150 = 50；one = -50 => 0
				Ephemeral5mCost: 50 * opus41C5m,
				CacheCreateCost: 50 * opus41C5m,
				TotalCost:       50 * opus41C5m,
				HasPricing:      true,
			},
		},
		{
			name:  "1h 单价走 sonnet 名称启发式（模型不在硬编码 1h 表内）",
			model: "claude-sonnet-4-5-20250929",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 1000,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral1hTokens: 1000},
			},
			want: modelpricing.CostBreakdown{
				Ephemeral1hCost: 1000 * sonnet45C1h,
				CacheCreateCost: 1000 * sonnet45C1h,
				TotalCost:       1000 * sonnet45C1h,
				HasPricing:      true,
			},
		},
		{
			// claude-opus-4-5-20251101 是内置 JSON 精确条目，但不在硬编码 1h 表内，
			// 走名称含 "opus" 的启发式分支（0.00003/token）。
			name:  "1h 单价走 opus 名称启发式（模型不在硬编码 1h 表内）",
			model: "claude-opus-4-5-20251101",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 1000,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral1hTokens: 1000},
			},
			want: modelpricing.CostBreakdown{
				Ephemeral1hCost: 1000 * 0.00003,
				CacheCreateCost: 1000 * 0.00003,
				TotalCost:       1000 * 0.00003,
				HasPricing:      true,
			},
		},
		{
			// claude-haiku-4-5 是内置 JSON 精确条目，但不在硬编码 1h 表内，
			// 走名称含 "haiku" 的启发式分支（0.0000016/token）。
			name:  "1h 单价走 haiku 名称启发式（模型不在硬编码 1h 表内）",
			model: "claude-haiku-4-5",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 1000,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral1hTokens: 1000},
			},
			want: modelpricing.CostBreakdown{
				Ephemeral1hCost: 1000 * 0.0000016,
				CacheCreateCost: 1000 * 0.0000016,
				TotalCost:       1000 * 0.0000016,
				HasPricing:      true,
			},
		},
		{
			name:  "cache 5m 明细为负且无差额回填：5m 钳位为 0，1h 保留",
			model: "claude-opus-4-1-20250805",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 0,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral5mTokens: -100, Ephemeral1hTokens: 200},
			},
			want: modelpricing.CostBreakdown{
				// remaining = 0-(-100)-200 = -100（不生效）=> five=-100 钳位为 0；one=200
				Ephemeral1hCost: 200 * opus41C1h,
				CacheCreateCost: 200 * opus41C1h,
				TotalCost:       200 * opus41C1h,
				HasPricing:      true,
			},
		},
		{
			name:  "非 claude 模型的 1h cache token 单价为 0（免费）",
			model: "ai21.j2-mid-v1",
			usage: modelpricing.UsageSnapshot{
				CacheCreateTokens: 1000,
				CacheCreation:     &modelpricing.CacheCreationDetail{Ephemeral1hTokens: 1000},
			},
			want: modelpricing.CostBreakdown{HasPricing: true},
		},
		{
			name:  "ensureCachePricing 推导：cache create=1.25x、read=0.1x input 单价",
			model: "ai21.j2-mid-v1",
			usage: modelpricing.UsageSnapshot{CacheCreateTokens: 1000, CacheReadTokens: 1000},
			want: modelpricing.CostBreakdown{
				Ephemeral5mCost: 1000 * j2C5m,
				CacheCreateCost: 1000 * j2C5m,
				CacheReadCost:   1000 * j2Read,
				TotalCost:       1000*j2C5m + 1000*j2Read,
				HasPricing:      true,
			},
		},
		{
			name:  "IsFast：所有分项与总额均乘 2",
			model: "claude-sonnet-4-5-20250929",
			usage: modelpricing.UsageSnapshot{
				InputTokens:       1000,
				OutputTokens:      500,
				CacheCreateTokens: 100,
				CacheReadTokens:   200,
				IsFast:            true,
			},
			want: modelpricing.CostBreakdown{
				InputCost:       2 * 1000 * sonnet45In,
				OutputCost:      2 * 500 * sonnet45Out,
				Ephemeral5mCost: 2 * 100 * sonnet45C5m,
				CacheCreateCost: 2 * 100 * sonnet45C5m,
				CacheReadCost:   2 * 200 * sonnet45Read,
				TotalCost:       2 * (1000*sonnet45In + 500*sonnet45Out + 100*sonnet45C5m + 200*sonnet45Read),
				HasPricing:      true,
			},
		},
		{
			name:  "long-context 边界：totalInput 恰好 200000 不触发长上下文",
			model: "claude-sonnet-4-20250514[1m]",
			usage: modelpricing.UsageSnapshot{InputTokens: 200000, OutputTokens: 10},
			want: modelpricing.CostBreakdown{
				InputCost:  200000 * sonnet4In,
				OutputCost: 10 * sonnet4Out,
				TotalCost:  200000*sonnet4In + 10*sonnet4Out,
				HasPricing: true,
			},
		},
		{
			name:  "long-context 边界：totalInput=200001 触发长上下文单价",
			model: "claude-sonnet-4-20250514[1m]",
			usage: modelpricing.UsageSnapshot{InputTokens: 200001, OutputTokens: 10},
			want: modelpricing.CostBreakdown{
				InputCost:     200001 * longIn,
				OutputCost:    10 * longOut,
				TotalCost:     200001*longIn + 10*longOut,
				HasPricing:    true,
				IsLongContext: true,
			},
		},
		{
			name:  "long-context 阈值计入 cache token；cache 费用仍用基础单价",
			model: "claude-sonnet-4-20250514[1m]",
			usage: modelpricing.UsageSnapshot{
				InputTokens:       100,
				CacheCreateTokens: 150000,
				CacheReadTokens:   50000,
				// totalInput = 100+150000+50000 = 200100 > 200000
			},
			want: modelpricing.CostBreakdown{
				InputCost:       100 * longIn,
				Ephemeral5mCost: 150000 * sonnet4C5m,
				CacheCreateCost: 150000 * sonnet4C5m,
				CacheReadCost:   50000 * sonnet4Read,
				TotalCost:       100*longIn + 150000*sonnet4C5m + 50000*sonnet4Read,
				HasPricing:      true,
				IsLongContext:   true,
			},
		},
		{
			name:  "未知模型但名称含 [1m] 且超阈值：兜底命中唯一长上下文条目，HasPricing 被置 true",
			model: "mystery-model[1m]",
			usage: modelpricing.UsageSnapshot{InputTokens: 300000, OutputTokens: 1000},
			want: modelpricing.CostBreakdown{
				InputCost:     300000 * longIn,
				OutputCost:    1000 * longOut,
				TotalCost:     300000*longIn + 1000*longOut,
				HasPricing:    true,
				IsLongContext: true,
			},
		},
		{
			name:  "未知模型含 [1m] 但恰在阈值 200000：不触发长上下文，全 0",
			model: "mystery-model[1m]",
			usage: modelpricing.UsageSnapshot{InputTokens: 200000},
			want:  modelpricing.CostBreakdown{},
		},
		{
			name:  "不含 [1m] 的模型即使超 200000 也不走长上下文",
			model: "claude-sonnet-4-5-20250929",
			usage: modelpricing.UsageSnapshot{InputTokens: 250000},
			want: modelpricing.CostBreakdown{
				InputCost:  250000 * sonnet45In,
				TotalCost:  250000 * sonnet45In,
				HasPricing: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.CalculateCost(tt.model, tt.usage)
			assertBreakdown(t, got, tt.want)
		})
	}
}

// TestGetPricingResolution 通过 CalculateCost 的 input 费用锁定 getPricing 的分级命中顺序。
// 所有用例的匹配结果均无歧义（精确/前缀剥离/归一化命中唯一 key；模糊命中为单一候选）。
func TestGetPricingResolution(t *testing.T) {
	svc := newService(t)

	const inputTokens = 100000 // < 200000，避免长上下文分支干扰

	tests := []struct {
		name          string
		model         string
		wantHas       bool
		wantInputCost float64
	}{
		{
			name:          "精确命中",
			model:         "claude-sonnet-4-5-20250929",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet45In,
		},
		{
			name:          "精确命中（不同单价的 opus，防止误配同价条目）",
			model:         "claude-opus-4-1-20250805",
			wantHas:       true,
			wantInputCost: inputTokens * opus41In,
		},
		{
			name:          "区域前缀剥离：us. 前缀",
			model:         "us.claude-sonnet-4-5-20250929",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet45In,
		},
		{
			name:          "区域前缀剥离大小写不敏感：US. 前缀",
			model:         "US.claude-sonnet-4-5-20250929",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet45In,
		},
		{
			name:          "区域 + anthropic. 提供商前缀剥离",
			model:         "us.anthropic.claude-sonnet-4-5",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet45In, // claude-sonnet-4-5 与 20250929 版同价
		},
		{
			name:          "normalize 归一化命中：大小写差异",
			model:         "Claude-Sonnet-4-5-20250929",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet45In,
		},
		{
			name:          "normalize 归一化命中：下划线分隔",
			model:         "claude_sonnet_4_5_20250929",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet45In,
		},
		{
			// [ ] 不在归一化替换字符集内，因此该名字只能走双向子串模糊匹配；
			// 内置表中唯一候选是 claude-sonnet-4-20250514（已核对）。
			name:          "双向子串模糊匹配：单一候选",
			model:         "claude-sonnet-4-20250514[1m]",
			wantHas:       true,
			wantInputCost: inputTokens * sonnet4In,
		},
		{
			name:          "完全未命中",
			model:         "totally-unknown-model-xyz-987",
			wantHas:       false,
			wantInputCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.CalculateCost(tt.model, modelpricing.UsageSnapshot{InputTokens: inputTokens})
			if got.HasPricing != tt.wantHas {
				t.Errorf("HasPricing = %v, want %v", got.HasPricing, tt.wantHas)
			}
			if !almostEqual(got.InputCost, tt.wantInputCost) {
				t.Errorf("InputCost = %.12f, want %.12f", got.InputCost, tt.wantInputCost)
			}
			if !almostEqual(got.TotalCost, tt.wantInputCost) {
				t.Errorf("TotalCost = %.12f, want %.12f", got.TotalCost, tt.wantInputCost)
			}
		})
	}
}

func TestCalculateCostNilServiceAndEmptyModel(t *testing.T) {
	usage := modelpricing.UsageSnapshot{InputTokens: 1000, OutputTokens: 1000}

	var nilSvc *modelpricing.Service
	assertBreakdown(t, nilSvc.CalculateCost("claude-sonnet-4-5-20250929", usage), modelpricing.CostBreakdown{})

	svc := newService(t)
	assertBreakdown(t, svc.CalculateCost("", usage), modelpricing.CostBreakdown{})
}

func TestDefaultServiceSingleton(t *testing.T) {
	first, err := modelpricing.DefaultService()
	if err != nil {
		t.Fatalf("DefaultService() error = %v", err)
	}
	if first == nil {
		t.Fatal("DefaultService() returned nil")
	}
	second, err := modelpricing.DefaultService()
	if err != nil {
		t.Fatalf("DefaultService() second call error = %v", err)
	}
	if first != second {
		t.Error("DefaultService() returned different instances")
	}

	got := first.CalculateCost("claude-sonnet-4-5-20250929", modelpricing.UsageSnapshot{InputTokens: 1000})
	if !got.HasPricing || !almostEqual(got.InputCost, 1000*sonnet45In) {
		t.Errorf("DefaultService pricing mismatch: HasPricing=%v InputCost=%.12f", got.HasPricing, got.InputCost)
	}
}

// TestNewServiceWithFile 使用 t.TempDir 内的文件验证外部定价合并，不接触用户目录。
func TestNewServiceWithFile(t *testing.T) {
	t.Run("路径不存在时回退为内置定价", func(t *testing.T) {
		svc, err := modelpricing.NewServiceWithFile(filepath.Join(t.TempDir(), "no-such-file.json"))
		if err != nil {
			t.Fatalf("NewServiceWithFile() error = %v", err)
		}
		got := svc.CalculateCost("claude-sonnet-4-5-20250929", modelpricing.UsageSnapshot{InputTokens: 1000})
		if !got.HasPricing || !almostEqual(got.InputCost, 1000*sonnet45In) {
			t.Errorf("builtin fallback mismatch: HasPricing=%v InputCost=%.12f", got.HasPricing, got.InputCost)
		}
	})

	t.Run("外部文件为非法 JSON 时静默忽略，保留内置定价", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.json")
		if err := os.WriteFile(path, []byte("{not-valid-json"), 0o644); err != nil {
			t.Fatalf("write broken pricing: %v", err)
		}
		svc, err := modelpricing.NewServiceWithFile(path)
		if err != nil {
			t.Fatalf("NewServiceWithFile() error = %v", err)
		}
		got := svc.CalculateCost("claude-sonnet-4-5-20250929", modelpricing.UsageSnapshot{InputTokens: 1000})
		if !got.HasPricing || !almostEqual(got.InputCost, 1000*sonnet45In) {
			t.Errorf("broken external file changed builtin pricing: HasPricing=%v InputCost=%.12f", got.HasPricing, got.InputCost)
		}
	})

	t.Run("外部条目覆盖内置定价并重新推导缓存单价", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pricing.json")
		external := `{"claude-sonnet-4-5-20250929": {"input_cost_per_token": 0.00001, "output_cost_per_token": 0.00002}}`
		if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
			t.Fatalf("write external pricing: %v", err)
		}
		svc, err := modelpricing.NewServiceWithFile(path)
		if err != nil {
			t.Fatalf("NewServiceWithFile() error = %v", err)
		}
		got := svc.CalculateCost("claude-sonnet-4-5-20250929", modelpricing.UsageSnapshot{
			InputTokens:       1000,
			OutputTokens:      1000,
			CacheCreateTokens: 1000,
			CacheReadTokens:   1000,
		})
		want := modelpricing.CostBreakdown{
			InputCost:  1000 * 0.00001,
			OutputCost: 1000 * 0.00002,
			// 外部条目未提供缓存单价，由 ensureCachePricing 按新 input 推导
			Ephemeral5mCost: 1000 * 0.00001 * 1.25,
			CacheCreateCost: 1000 * 0.00001 * 1.25,
			CacheReadCost:   1000 * 0.00001 * 0.1,
			TotalCost:       1000*0.00001 + 1000*0.00002 + 1000*0.00001*1.25 + 1000*0.00001*0.1,
			HasPricing:      true,
		}
		assertBreakdown(t, got, want)
	})

	t.Run("外部文件新增模型可被精确命中", func(t *testing.T) {
		const custom = "my-custom-model-abc-777"

		base := newService(t)
		if got := base.CalculateCost(custom, modelpricing.UsageSnapshot{InputTokens: 1000}); got.HasPricing {
			t.Fatalf("model %q unexpectedly present in builtin table", custom)
		}

		path := filepath.Join(t.TempDir(), "pricing.json")
		external := `{"` + custom + `": {"input_cost_per_token": 0.000002, "output_cost_per_token": 0.000004}}`
		if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
			t.Fatalf("write external pricing: %v", err)
		}
		svc, err := modelpricing.NewServiceWithFile(path)
		if err != nil {
			t.Fatalf("NewServiceWithFile() error = %v", err)
		}
		got := svc.CalculateCost(custom, modelpricing.UsageSnapshot{InputTokens: 1000, OutputTokens: 500})
		want := modelpricing.CostBreakdown{
			InputCost:  1000 * 0.000002,
			OutputCost: 500 * 0.000004,
			TotalCost:  1000*0.000002 + 500*0.000004,
			HasPricing: true,
		}
		assertBreakdown(t, got, want)
	})
}
