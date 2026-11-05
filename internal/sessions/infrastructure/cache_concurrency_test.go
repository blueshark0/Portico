package infrastructure

import (
	"sync"
	"testing"
	"time"

	sessionapp "portico/internal/sessions/application"
)

type blockingSessionRepo struct {
	sessionapp.Repository
	mu        sync.Mutex
	providers map[string]string
	before    func(operation, sessionID string)
}

func (r *blockingSessionRepo) GetSessionProvider(_, id string) (string, error) {
	r.before("read", id)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.providers[id], nil
}
func (r *blockingSessionRepo) BindSessionToProvider(_, id, provider string) error {
	r.before("bind", id)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[id] = provider
	return nil
}
func (r *blockingSessionRepo) UpdateSessionSuccess(_, id string) error {
	r.before("update", id)
	return nil
}
func (r *blockingSessionRepo) UnbindSession(_, id string) error {
	r.before("unbind", id)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, id)
	return nil
}

func awaitCacheOperation(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session operation blocked behind unrelated repository I/O")
	}
}

func TestCacheRepositoryIODoesNotBlockOtherSessions(t *testing.T) {
	for _, operation := range []string{"read", "bind", "update"} {
		t.Run(operation, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			repo := &blockingSessionRepo{providers: map[string]string{"slow": "alpha", "cold": "beta"}, before: func(op, id string) {
				if op == operation && id == "slow" {
					close(entered)
					<-release
				}
			}}
			cache := NewCache(sessionapp.NewService(repo))
			if err := cache.BindSessionToProvider("claude", "hot", "beta"); err != nil {
				t.Fatal(err)
			}
			slowDone := make(chan error, 1)
			go func() {
				switch operation {
				case "read":
					_, err := cache.GetSessionProvider("claude", "slow")
					slowDone <- err
				case "bind":
					slowDone <- cache.BindSessionToProvider("claude", "slow", "alpha")
				case "update":
					slowDone <- cache.UpdateSessionSuccess("claude", "slow")
				}
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("repository operation did not start")
			}
			for _, id := range []string{"hot", "cold"} {
				done := make(chan error, 1)
				go func() { _, err := cache.GetSessionProvider("claude", id); done <- err }()
				awaitCacheOperation(t, done)
			}
			unblock()
			awaitCacheOperation(t, slowDone)
		})
	}
}

func TestCacheUnbindSerializesWithInFlightWrite(t *testing.T) {
	for _, operation := range []string{"bind", "update"} {
		t.Run(operation, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			repo := &blockingSessionRepo{providers: map[string]string{"s": "alpha"}, before: func(op, _ string) {
				if op == operation {
					close(entered)
					<-release
				}
			}}
			service := sessionapp.NewService(repo)
			cache := NewCache(service)
			_, generation, err := cache.GetSessionProviderSnapshot("claude", "s")
			if err != nil {
				t.Fatal(err)
			}
			writeDone := make(chan error, 1)
			go func() {
				if operation == "bind" {
					writeDone <- cache.BindSessionToProviderGeneration("claude", "s", "beta", generation)
				} else {
					writeDone <- cache.UpdateSessionSuccessGeneration("claude", "s", generation)
				}
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("write did not start")
			}
			unbindDone := make(chan error, 1)
			go func() { unbindDone <- service.Unbind("claude", "s") }()
			select {
			case err := <-unbindDone:
				t.Fatalf("unbind overtook in-flight write: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			unblock()
			awaitCacheOperation(t, writeDone)
			awaitCacheOperation(t, unbindDone)
			// A delayed response from before Unbind must not recreate the DB binding.
			if err := cache.BindSessionToProviderGeneration("claude", "s", "stale", generation); err != nil {
				t.Fatal(err)
			}
			if err := cache.UpdateSessionSuccessGeneration("claude", "s", generation); err != nil {
				t.Fatal(err)
			}
			if got, err := cache.GetSessionProvider("claude", "s"); err != nil || got != "" {
				t.Fatalf("binding restored: %q, %v", got, err)
			}
			if got, _ := repo.GetSessionProvider("claude", "s"); got != "" {
				t.Fatalf("database binding restored: %q", got)
			}
		})
	}
}

func TestCacheReadDoesNotPublishAfterInvalidation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	repo := &blockingSessionRepo{providers: map[string]string{"s": "old"}, before: func(op, _ string) {
		if op == "read" {
			close(entered)
			<-release
		}
	}}
	cache := NewCache(sessionapp.NewService(repo))
	done := make(chan error, 1)
	var provider string
	go func() { var err error; provider, _, err = cache.GetSessionProviderSnapshot("claude", "s"); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("read did not start")
	}
	cache.InvalidateSession("claude", "s")
	once.Do(func() { close(release) })
	awaitCacheOperation(t, done)
	if provider != "" {
		t.Fatalf("stale provider published: %q", provider)
	}
}
