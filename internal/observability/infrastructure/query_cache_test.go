package infrastructure

import (
	"testing"
	"time"
)

func TestTTLCacheReturnsClonedValue(t *testing.T) {
	cache := newTTLCache(50*time.Millisecond, func(input []string) []string {
		return append([]string(nil), input...)
	})
	cache.Set("providers", []string{"a", "b"})

	first, ok := cache.Get("providers")
	if !ok {
		t.Fatal("expected cached value")
	}
	first[0] = "changed"

	second, ok := cache.Get("providers")
	if !ok {
		t.Fatal("expected cached value on second read")
	}
	if second[0] != "a" {
		t.Fatalf("expected cached value to be cloned, got %v", second)
	}
}

func TestTTLCacheExpires(t *testing.T) {
	cache := newTTLCache(10*time.Millisecond, func(input string) string { return input })
	cache.Set("key", "value")
	time.Sleep(20 * time.Millisecond)

	if _, ok := cache.Get("key"); ok {
		t.Fatal("expected cache entry to expire")
	}
}

func TestVersionedTTLCacheInvalidatesOnVersionChange(t *testing.T) {
	var version uint64
	cache := newVersionedTTLCache(time.Minute, func(input string) string { return input }, func() uint64 { return version })

	cache.SetVersion("key", "v0", cache.Version())
	if got, ok := cache.Get("key"); !ok || got != "v0" {
		t.Fatalf("expected cached v0, got %q ok=%v", got, ok)
	}

	version++
	if _, ok := cache.Get("key"); ok {
		t.Fatal("expected entry to be invalidated after data version changed")
	}
}

func TestVersionedTTLCacheRejectsResultComputedBeforeWrite(t *testing.T) {
	var version uint64
	cache := newVersionedTTLCache(time.Minute, func(input string) string { return input }, func() uint64 { return version })

	// 查询开始前读到版本 0，查询期间发生写入，结果不能被当作最新缓存。
	before := cache.Version()
	version++
	cache.SetVersion("key", "stale", before)
	if _, ok := cache.Get("key"); ok {
		t.Fatal("expected result computed before a concurrent write to be rejected")
	}
}
