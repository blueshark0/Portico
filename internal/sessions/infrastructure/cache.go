package infrastructure

import (
	"fmt"
	"log"
	"sync"
	"time"

	sessionapp "portico/internal/sessions/application"
	"portico/internal/shared/kernel"

	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	sessionCacheSize       = 1000
	sessionPersistInterval = 60 * time.Second
)

type sessionCacheEntry struct {
	providerName     string
	lastSuccessAt    time.Time
	lastPersistedAt  time.Time
	pendingPersistAt time.Time
	generation       uint64
}

// Cache 的 entry 会被 gin 请求 goroutine、sessionUpdateWorker 后台 goroutine
// 与 Wails UI goroutine（Unbind→InvalidateSession）并发读写；lru.Cache 只保证
// 自身 Get/Add 线程安全，entry 字段的读改写必须由 mu 统一串行化。
// mu 只保护内存状态；数据库 I/O 在锁外执行，并通过 Service 的会话锁与解绑串行化。
type Cache struct {
	mu      sync.Mutex
	cache   *lru.Cache[string, *sessionCacheEntry]
	service *sessionapp.Service
}

func NewCache(service *sessionapp.Service) *Cache {
	cache, err := lru.New[string, *sessionCacheEntry](sessionCacheSize)
	if err != nil {
		log.Printf("[ERROR] 创建会话缓存失败: %v\n", err)
		return &Cache{service: service}
	}
	sc := &Cache{
		cache:   cache,
		service: service,
	}
	if service != nil {
		service.RegisterCache(sc)
	}
	return sc
}

func (sc *Cache) cacheKey(platform, sessionID string) string {
	return fmt.Sprintf("%s:%s", platform, sessionID)
}

func (sc *Cache) GetSessionProvider(platform, sessionID string) (string, error) {
	providerName, _, err := sc.GetSessionProviderSnapshot(platform, sessionID)
	return providerName, err
}

func (sc *Cache) GetSessionProviderSnapshot(platform, sessionID string) (string, uint64, error) {
	if sessionID == "" {
		return "", 0, nil
	}
	// Hits never wait for repository I/O, even for another session's write.
	sc.mu.Lock()
	providerName, generation, hit := sc.cachedProviderLocked(platform, sessionID)
	sc.mu.Unlock()
	if hit || sc.service == nil {
		return providerName, generation, nil
	}
	unlock := sc.service.LockSession(platform, sessionID)
	defer unlock()
	sc.mu.Lock()
	providerName, generation, hit = sc.cachedProviderLocked(platform, sessionID)
	sc.mu.Unlock()
	if hit {
		return providerName, generation, nil
	}
	providerName, err := sc.service.GetSessionProvider(platform, sessionID)
	sc.mu.Lock()
	defer sc.mu.Unlock()
	// Direct invalidation or a success recorded during the query takes priority.
	currentProvider, currentGeneration, currentHit := sc.cachedProviderLocked(platform, sessionID)
	if currentGeneration != generation || currentHit {
		return currentProvider, currentGeneration, nil
	}
	if err != nil {
		return "", generation, err
	}
	if providerName != "" && sc.cache != nil {
		sc.cache.Add(sc.cacheKey(platform, sessionID), &sessionCacheEntry{
			providerName:  providerName,
			lastSuccessAt: time.Now(),
			generation:    generation,
		})
	}
	return providerName, generation, nil
}

func (sc *Cache) cachedProviderLocked(platform, sessionID string) (string, uint64, bool) {
	if sc.cache != nil {
		if entry, ok := sc.cache.Get(sc.cacheKey(platform, sessionID)); ok {
			if !sc.isExpired(platform, entry.lastSuccessAt) {
				return entry.providerName, entry.generation, true
			}
			entry.providerName = ""
			entry.lastSuccessAt = time.Time{}
			entry.lastPersistedAt = time.Time{}
			entry.pendingPersistAt = time.Time{}
			return "", entry.generation, false
		}
	}
	return "", 0, false
}

func (sc *Cache) BindSessionToProvider(platform, sessionID, providerName string) error {
	sc.mu.Lock()
	generation := sc.sessionGenerationLocked(platform, sessionID)
	sc.mu.Unlock()
	return sc.BindSessionToProviderGeneration(platform, sessionID, providerName, generation)
}

func (sc *Cache) BindSessionToProviderGeneration(platform, sessionID, providerName string, generation uint64) error {
	if sessionID == "" || providerName == "" {
		return nil
	}
	if sc.service != nil {
		unlock := sc.service.LockSession(platform, sessionID)
		defer unlock()
	}
	if !sc.IsCurrentGeneration(platform, sessionID, generation) {
		return nil
	}
	var err error
	if sc.service != nil {
		err = sc.service.BindSessionToProvider(platform, sessionID, providerName)
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if !sc.isCurrentGenerationLocked(platform, sessionID, generation) {
		return err
	}
	if err != nil {
		return err
	}
	if sc.cache != nil {
		key := sc.cacheKey(platform, sessionID)
		now := time.Now()
		sc.cache.Add(key, &sessionCacheEntry{
			providerName:    providerName,
			lastSuccessAt:   now,
			lastPersistedAt: now,
			generation:      generation,
		})
	}
	return nil
}

func (sc *Cache) UpdateSessionSuccess(platform, sessionID string) error {
	sc.mu.Lock()
	generation := sc.sessionGenerationLocked(platform, sessionID)
	sc.mu.Unlock()
	return sc.UpdateSessionSuccessGeneration(platform, sessionID, generation)
}

func (sc *Cache) UpdateSessionSuccessGeneration(platform, sessionID string, generation uint64) error {
	if sessionID == "" {
		return nil
	}
	if sc.service != nil {
		unlock := sc.service.LockSession(platform, sessionID)
		defer unlock()
	}
	if !sc.IsCurrentGeneration(platform, sessionID, generation) {
		return nil
	}
	var err error
	if sc.service != nil {
		err = sc.service.UpdateSessionSuccess(platform, sessionID)
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if !sc.isCurrentGenerationLocked(platform, sessionID, generation) {
		return err
	}
	if err != nil {
		sc.clearPendingSessionSuccessLocked(platform, sessionID)
		return err
	}
	if sc.cache != nil {
		now := time.Now()
		key := sc.cacheKey(platform, sessionID)
		if entry, ok := sc.cache.Get(key); ok {
			entry.lastSuccessAt = now
			entry.lastPersistedAt = now
			entry.pendingPersistAt = time.Time{}
		}
	}
	return nil
}

func (sc *Cache) RecordSessionSuccess(platform, sessionID, providerName string) bool {
	sc.mu.Lock()
	generation := sc.sessionGenerationLocked(platform, sessionID)
	sc.mu.Unlock()
	_, persist := sc.RecordSessionSuccessGeneration(platform, sessionID, providerName, generation)
	return persist
}

// RecordSessionSuccessGeneration 分别返回 generation 是否仍有效、是否需要落盘。
// 展示状态等按请求更新的逻辑使用第一个返回值，不受持久化节流影响。
func (sc *Cache) RecordSessionSuccessGeneration(platform, sessionID, providerName string, generation uint64) (bool, bool) {
	if sessionID == "" {
		return false, false
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cache == nil {
		return true, true
	}
	if !sc.isCurrentGenerationLocked(platform, sessionID, generation) {
		return false, false
	}
	now := time.Now()
	key := sc.cacheKey(platform, sessionID)
	entry, ok := sc.cache.Get(key)
	if !ok || entry == nil {
		entry = &sessionCacheEntry{providerName: providerName, generation: generation}
		sc.cache.Add(key, entry)
	}
	if providerName != "" {
		entry.providerName = providerName
	}
	entry.lastSuccessAt = now

	referenceTime := entry.lastPersistedAt
	if entry.pendingPersistAt.After(referenceTime) {
		referenceTime = entry.pendingPersistAt
	}
	if !referenceTime.IsZero() && now.Sub(referenceTime) < sessionPersistInterval {
		return true, false
	}
	entry.pendingPersistAt = now
	return true, true
}

func (sc *Cache) InvalidateSession(platform, sessionID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cache == nil {
		return
	}
	key := sc.cacheKey(platform, sessionID)
	entry, ok := sc.cache.Get(key)
	if !ok || entry == nil {
		sc.cache.Add(key, &sessionCacheEntry{generation: 1})
		return
	}
	entry.generation++
	entry.providerName = ""
	entry.lastSuccessAt = time.Time{}
	entry.lastPersistedAt = time.Time{}
	entry.pendingPersistAt = time.Time{}
}

func (sc *Cache) isExpired(platform string, lastSuccessAt time.Time) bool {
	timeout := kernel.Platform(platform).SessionTimeout()
	return time.Since(lastSuccessAt) > timeout
}

func (sc *Cache) ClearPendingSessionSuccess(platform, sessionID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.clearPendingSessionSuccessLocked(platform, sessionID)
}

func (sc *Cache) clearPendingSessionSuccessLocked(platform, sessionID string) {
	if sc.cache == nil || sessionID == "" {
		return
	}
	key := sc.cacheKey(platform, sessionID)
	entry, ok := sc.cache.Get(key)
	if !ok || entry == nil {
		return
	}
	entry.pendingPersistAt = time.Time{}
}

func (sc *Cache) SessionGeneration(platform, sessionID string) uint64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.sessionGenerationLocked(platform, sessionID)
}

func (sc *Cache) sessionGenerationLocked(platform, sessionID string) uint64 {
	if sc.cache == nil || sessionID == "" {
		return 0
	}
	key := sc.cacheKey(platform, sessionID)
	if entry, ok := sc.cache.Get(key); ok && entry != nil {
		return entry.generation
	}
	return 0
}

func (sc *Cache) IsCurrentGeneration(platform, sessionID string, generation uint64) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.isCurrentGenerationLocked(platform, sessionID, generation)
}

func (sc *Cache) isCurrentGenerationLocked(platform, sessionID string, generation uint64) bool {
	if sc.cache == nil || sessionID == "" {
		return true
	}
	return sc.sessionGenerationLocked(platform, sessionID) == generation
}

func (sc *Cache) Stats() (size int, capacity int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cache == nil {
		return 0, 0
	}
	return sc.cache.Len(), sessionCacheSize
}
