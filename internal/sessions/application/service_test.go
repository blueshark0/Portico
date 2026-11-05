package application

import (
	"sync"
	"testing"
	"time"

	sessiondomain "portico/internal/sessions/domain"
)

type sessionRepoStub struct {
	unbindCalls int
	lastSession string
	lastPlat    string
}

func (s *sessionRepoStub) GetSessionProvider(platform, sessionID string) (string, error) {
	return "", nil
}

func (s *sessionRepoStub) BindSessionToProvider(platform, sessionID, providerName string) error {
	return nil
}

func (s *sessionRepoStub) UpdateSessionSuccess(platform, sessionID string) error {
	return nil
}

func (s *sessionRepoStub) RecordSessionBackground(platform, sessionID string, background bool) error {
	return nil
}

func (s *sessionRepoStub) GetProviderSessions(platform, providerName string) ([]sessiondomain.SessionBinding, error) {
	return nil, nil
}

func (s *sessionRepoStub) GetPlatformSessions(platform string) ([]sessiondomain.SessionBinding, error) {
	return nil, nil
}

func (s *sessionRepoStub) UnbindSession(platform, sessionID string) error {
	s.unbindCalls++
	s.lastPlat = platform
	s.lastSession = sessionID
	return nil
}

func (s *sessionRepoStub) CleanExpiredSessions() error {
	return nil
}

type cacheStub struct {
	invalidations []string
}

func (c *cacheStub) InvalidateSession(platform, sessionID string) {
	c.invalidations = append(c.invalidations, platform+":"+sessionID)
}

func TestService_UnbindInvalidatesCaches(t *testing.T) {
	repo := &sessionRepoStub{}
	cache := &cacheStub{}
	service := NewService(repo)
	service.RegisterCache(cache)

	if err := service.Unbind("claude", "session-1"); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if repo.unbindCalls != 1 {
		t.Fatalf("expected repo unbind once, got %d", repo.unbindCalls)
	}
	if len(cache.invalidations) != 1 || cache.invalidations[0] != "claude:session-1" {
		t.Fatalf("unexpected cache invalidations: %#v", cache.invalidations)
	}
}

type cleanupServiceStub struct {
	mu    sync.Mutex
	calls int
}

func (s *cleanupServiceStub) CleanExpiredSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return nil
}

func (s *cleanupServiceStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestCleanupRunner_StartStop(t *testing.T) {
	service := &cleanupServiceStub{}
	runner := NewCleanupRunner(service, 10*time.Millisecond)
	runner.Start()
	t.Cleanup(runner.Stop)

	for i := 0; i < 50; i++ {
		if service.callCount() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected cleanup runner to invoke service at least once")
}

func TestSessionLocksReleaseIdleEntries(t *testing.T) {
	service := NewService(&sessionRepoStub{})
	unlockFirst := service.LockSession("claude", "s")
	unlockOther := service.LockSession("codex", "s")
	unlockOther()
	acquired, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		unlock := service.LockSession("claude", "s")
		close(acquired)
		<-release
		unlock()
		close(done)
	}()
	unlockFirst()
	<-acquired
	service.sessionLocksMu.Lock()
	remaining := len(service.sessionLocks)
	service.sessionLocksMu.Unlock()
	close(release)
	<-done
	if remaining != 1 || len(service.sessionLocks) != 0 {
		t.Fatalf("lock registry must retain active entries only: active=%d idle=%d", remaining, len(service.sessionLocks))
	}
}
