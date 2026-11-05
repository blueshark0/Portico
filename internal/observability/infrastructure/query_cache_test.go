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
