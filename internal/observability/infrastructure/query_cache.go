package infrastructure

import (
	"sync"
	"time"
)

type ttlCacheEntry[T any] struct {
	value     T
	expiresAt time.Time
	version   uint64
}

// ttlCache 是按 key 的查询结果缓存。可选的 version 函数提供数据版本号：
// 条目记录的是开始查询前读到的版本，版本变化（有新数据写入）后条目立即失效，
// TTL 只作为兜底。
type ttlCache[T any] struct {
	ttl     time.Duration
	clone   func(T) T
	version func() uint64

	mu     sync.RWMutex
	values map[string]ttlCacheEntry[T]
}

func newTTLCache[T any](ttl time.Duration, clone func(T) T) *ttlCache[T] {
	return newVersionedTTLCache(ttl, clone, nil)
}

func newVersionedTTLCache[T any](ttl time.Duration, clone func(T) T, version func() uint64) *ttlCache[T] {
	return &ttlCache[T]{
		ttl:     ttl,
		clone:   clone,
		version: version,
		values:  make(map[string]ttlCacheEntry[T]),
	}
}

// Version 返回当前数据版本；调用方须在查询数据库之前读取并传给 SetVersion，
// 这样查询期间发生的写入会让刚写入的条目直接失效，而不是被当成最新结果。
func (c *ttlCache[T]) Version() uint64 {
	if c == nil || c.version == nil {
		return 0
	}
	return c.version()
}

func (c *ttlCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil || c.ttl <= 0 {
		return zero, false
	}
	now := time.Now()
	version := c.Version()

	c.mu.RLock()
	entry, ok := c.values[key]
	c.mu.RUnlock()
	if !ok || now.After(entry.expiresAt) || entry.version != version {
		if ok {
			c.mu.Lock()
			delete(c.values, key)
			c.mu.Unlock()
		}
		return zero, false
	}
	return c.clone(entry.value), true
}

func (c *ttlCache[T]) Set(key string, value T) T {
	return c.SetVersion(key, value, c.Version())
}

func (c *ttlCache[T]) SetVersion(key string, value T, version uint64) T {
	if c == nil || c.ttl <= 0 {
		return value
	}
	cloned := c.clone(value)
	now := time.Now()
	current := c.Version()
	c.mu.Lock()
	// 顺带清掉过期/失效条目：key 含日期边界，旧 key 不会再被 Get 到。
	for k, entry := range c.values {
		if now.After(entry.expiresAt) || entry.version != current {
			delete(c.values, k)
		}
	}
	c.values[key] = ttlCacheEntry[T]{
		value:     cloned,
		expiresAt: now.Add(c.ttl),
		version:   version,
	}
	c.mu.Unlock()
	return c.clone(cloned)
}
