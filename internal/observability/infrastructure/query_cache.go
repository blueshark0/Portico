package infrastructure

import (
	"sync"
	"time"
)

type ttlCacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

type ttlCache[T any] struct {
	ttl   time.Duration
	clone func(T) T

	mu     sync.RWMutex
	values map[string]ttlCacheEntry[T]
}

func newTTLCache[T any](ttl time.Duration, clone func(T) T) *ttlCache[T] {
	return &ttlCache[T]{
		ttl:    ttl,
		clone:  clone,
		values: make(map[string]ttlCacheEntry[T]),
	}
}

func (c *ttlCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil || c.ttl <= 0 {
		return zero, false
	}
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.values[key]
	c.mu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
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
	if c == nil || c.ttl <= 0 {
		return value
	}
	cloned := c.clone(value)
	c.mu.Lock()
	c.values[key] = ttlCacheEntry[T]{
		value:     cloned,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
	return c.clone(cloned)
}
