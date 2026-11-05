package infrastructure

import (
	"testing"
	"time"
)

func TestCacheRecordSessionSuccessThrottlesPendingUpdates(t *testing.T) {
	cache := NewCache(nil)

	if !cache.RecordSessionSuccess("codex", "session-1", "alpha") {
		t.Fatal("expected first success to schedule persistence")
	}
	if cache.RecordSessionSuccess("codex", "session-1", "alpha") {
		t.Fatal("expected pending success not to schedule again")
	}

	cache.ClearPendingSessionSuccess("codex", "session-1")

	if !cache.RecordSessionSuccess("codex", "session-1", "alpha") {
		t.Fatal("expected cleared pending flag to allow reschedule")
	}
}

func TestCacheRecordSessionSuccessRespectsPersistInterval(t *testing.T) {
	cache := NewCache(nil)
	key := cache.cacheKey("claude", "session-2")
	cache.cache.Add(key, &sessionCacheEntry{
		providerName:    "beta",
		lastPersistedAt: timeNow().Add(-30 * time.Second),
	})

	current, persist := cache.RecordSessionSuccessGeneration("claude", "session-2", "beta", 0)
	if !current || persist {
		t.Fatal("expected throttled success to remain current without scheduling persistence")
	}

	entry, ok := cache.cache.Get(key)
	if !ok || entry == nil {
		t.Fatal("expected cache entry to remain present")
	}
	entry.lastPersistedAt = timeNow().Add(-sessionPersistInterval - time.Second)

	if !cache.RecordSessionSuccess("claude", "session-2", "beta") {
		t.Fatal("expected expired persisted entry to schedule update")
	}
}

func TestCacheInvalidationAdvancesGenerationAndSkipsStaleSuccess(t *testing.T) {
	cache := NewCache(nil)
	if err := cache.BindSessionToProviderGeneration("claude", "session-3", "alpha", 0); err != nil {
		t.Fatalf("bind session: %v", err)
	}

	_, generation, err := cache.GetSessionProviderSnapshot("claude", "session-3")
	if err != nil {
		t.Fatalf("get session snapshot: %v", err)
	}
	cache.InvalidateSession("claude", "session-3")

	current, persist := cache.RecordSessionSuccessGeneration("claude", "session-3", "alpha", generation)
	if current || persist {
		t.Fatal("expected stale success to be ignored after invalidation")
	}
	if got, err := cache.GetSessionProvider("claude", "session-3"); err != nil || got != "" {
		t.Fatalf("expected invalidated session to remain unbound, got provider=%q err=%v", got, err)
	}

	nextGeneration := cache.SessionGeneration("claude", "session-3")
	if nextGeneration == generation {
		t.Fatal("expected invalidation to advance generation")
	}
	if err := cache.BindSessionToProviderGeneration("claude", "session-3", "beta", nextGeneration); err != nil {
		t.Fatalf("rebind session: %v", err)
	}
	if got, err := cache.GetSessionProvider("claude", "session-3"); err != nil || got != "beta" {
		t.Fatalf("expected current generation rebind, got provider=%q err=%v", got, err)
	}
}

func timeNow() time.Time {
	return time.Now()
}
