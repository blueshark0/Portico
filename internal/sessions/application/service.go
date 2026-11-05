package application

import (
	"fmt"
	"sync"

	sessiondomain "portico/internal/sessions/domain"
)

type Repository interface {
	GetSessionProvider(platform, sessionID string) (string, error)
	BindSessionToProvider(platform, sessionID, providerName string) error
	UpdateSessionSuccess(platform, sessionID string) error
	RecordSessionBackground(platform, sessionID string, background bool) error
	GetProviderSessions(platform, providerName string) ([]sessiondomain.SessionBinding, error)
	GetPlatformSessions(platform string) ([]sessiondomain.SessionBinding, error)
	UnbindSession(platform, sessionID string) error
	CleanExpiredSessions() error
}

type CacheInvalidator interface {
	InvalidateSession(platform, sessionID string)
}

type Service struct {
	repo           Repository
	sessionLocksMu sync.Mutex
	sessionLocks   map[[2]string]*sessionLock

	cachesMu sync.Mutex
	caches   []CacheInvalidator
}

type sessionLock struct {
	mu   sync.Mutex
	refs int
}

// LockSession serializes a cache's generation check, repository operation and
// publication with Unbind. Never acquire it while holding a cache mutex.
// Entries include waiters and are removed when the last caller releases them.
func (s *Service) LockSession(platform, sessionID string) func() {
	key := [2]string{platform, sessionID}
	s.sessionLocksMu.Lock()
	if s.sessionLocks == nil {
		s.sessionLocks = make(map[[2]string]*sessionLock)
	}
	lock := s.sessionLocks[key]
	if lock == nil {
		lock = &sessionLock{}
		s.sessionLocks[key] = lock
	}
	lock.refs++
	s.sessionLocksMu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.sessionLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.sessionLocks, key)
		}
		s.sessionLocksMu.Unlock()
	}
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetSessionProvider(platform, sessionID string) (string, error) {
	return s.repo.GetSessionProvider(platform, sessionID)
}

func (s *Service) BindSessionToProvider(platform, sessionID, providerName string) error {
	return s.repo.BindSessionToProvider(platform, sessionID, providerName)
}

func (s *Service) UpdateSessionSuccess(platform, sessionID string) error {
	return s.repo.UpdateSessionSuccess(platform, sessionID)
}

// RecordSessionBackground 只更新展示可见性，不改变路由绑定。仓储保证主会话
// 请求具有优先性：同一 session-id 的后台请求不能重新隐藏已确认的主会话。
func (s *Service) RecordSessionBackground(platform, sessionID string, background bool) error {
	if sessionID == "" {
		return nil
	}
	unlock := s.LockSession(platform, sessionID)
	defer unlock()
	return s.repo.RecordSessionBackground(platform, sessionID, background)
}

func (s *Service) ListByPlatform(platform string) ([]sessiondomain.SessionBinding, error) {
	return s.repo.GetPlatformSessions(platform)
}

func (s *Service) ListByProvider(platform, providerName string) ([]sessiondomain.SessionBinding, error) {
	return s.repo.GetProviderSessions(platform, providerName)
}

func (s *Service) Unbind(platform, sessionID string) error {
	if platform == "" || sessionID == "" {
		return fmt.Errorf("平台和会话ID不能为空")
	}
	unlock := s.LockSession(platform, sessionID)
	defer unlock()
	if err := s.repo.UnbindSession(platform, sessionID); err != nil {
		return err
	}
	s.invalidateCaches(platform, sessionID)
	return nil
}

func (s *Service) CleanExpiredSessions() error {
	return s.repo.CleanExpiredSessions()
}

func (s *Service) RegisterCache(cache CacheInvalidator) {
	if cache == nil {
		return
	}
	s.cachesMu.Lock()
	defer s.cachesMu.Unlock()
	s.caches = append(s.caches, cache)
}

func (s *Service) invalidateCaches(platform, sessionID string) {
	s.cachesMu.Lock()
	defer s.cachesMu.Unlock()
	for _, cache := range s.caches {
		if cache != nil {
			cache.InvalidateSession(platform, sessionID)
		}
	}
}
