package modelpricing

import (
	"sync"
	"testing"
)

func TestFuzzyPricingIsDeterministicAcrossServices(t *testing.T) {
	const model = "zz-deterministic-alias"
	external := map[string]PricingEntry{
		"zz-provider/" + model + "-pro": {InputCostPerToken: 1},
		"aa-provider/" + model + "-pro": {InputCostPerToken: 2},
	}
	for i := 0; i < 8; i++ {
		svc, err := NewService()
		if err != nil {
			t.Fatal(err)
		}
		svc.mergeExternal(external)
		for j := 0; j < 10; j++ {
			got := svc.CalculateCost(model, UsageSnapshot{InputTokens: 1})
			// 候选按 normalized key 排序，aa-provider 必须稳定优先。
			if got.TotalCost != 2 || !got.HasPricing {
				t.Fatalf("第 %d 个服务第 %d 次结果 = %+v, want stable aa-provider price", i, j, got)
			}
		}
	}
}

func TestNormalizedPricingCollisionIsDeterministic(t *testing.T) {
	// 两个原始 key 规范化后相同；查询使用空格形式，避免走精确 key 命中。
	external := map[string]PricingEntry{
		"zz-provider/foo-bar": {InputCostPerToken: 3},
		"zz_provider/foo_bar": {InputCostPerToken: 4},
	}
	for i := 0; i < 8; i++ {
		svc, err := NewService()
		if err != nil {
			t.Fatal(err)
		}
		svc.mergeExternal(external)
		got := svc.CalculateCost("zz provider foo bar", UsageSnapshot{InputTokens: 1})
		if got.TotalCost != 3 {
			t.Fatalf("normalized 冲突结果 = %v, want deterministic first key price 3", got.TotalCost)
		}
	}
}

func TestPricingLookupCacheInvalidatedByMerge(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatal(err)
	}
	const model = "zz-no-such-price-9f8e7d6c"
	usage := UsageSnapshot{InputTokens: 2}
	if got := svc.CalculateCost(model, usage); got.HasPricing {
		t.Fatalf("新模型初始结果 = %+v, want no pricing", got)
	}

	svc.mergeExternal(map[string]PricingEntry{
		model: {InputCostPerToken: 3},
	})
	if got := svc.CalculateCost(model, usage); got.TotalCost != 6 || !got.HasPricing {
		t.Fatalf("合并新模型后结果 = %+v, want cost 6", got)
	}

	svc.mergeExternal(map[string]PricingEntry{
		model: {InputCostPerToken: 5},
	})
	if got := svc.CalculateCost(model, usage); got.TotalCost != 10 {
		t.Fatalf("覆盖旧价格后结果 = %+v, want cost 10", got)
	}
}

func TestCalculateCostConcurrentWithExternalMerge(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatal(err)
	}
	const model = "zz-concurrent-pricing"
	svc.mergeExternal(map[string]PricingEntry{model: {InputCostPerToken: 1}})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = svc.CalculateCost(model, UsageSnapshot{InputTokens: 2, CacheReadTokens: 4})
			}
		}()
	}
	for i := 0; i < 30; i++ {
		svc.mergeExternal(map[string]PricingEntry{model: {InputCostPerToken: float64(i%3 + 1)}})
	}
	wg.Wait()
	if got := svc.CalculateCost(model, UsageSnapshot{InputTokens: 2}); !got.HasPricing {
		t.Fatal("并发合并后模型仍应有价格")
	}
}
