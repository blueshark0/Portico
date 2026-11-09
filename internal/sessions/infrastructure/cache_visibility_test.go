package infrastructure

import (
	"sync"
	"testing"

	sessionapp "portico/internal/sessions/application"
)

// countingVisibilityRepo 包装真实 SQLite 仓储，统计可见性写库次数。
type countingVisibilityRepo struct {
	*SQLiteRepository
	mu     sync.Mutex
	writes int
}

func (r *countingVisibilityRepo) RecordSessionBackground(platform, sessionID string, background bool) error {
	r.mu.Lock()
	r.writes++
	r.mu.Unlock()
	return r.SQLiteRepository.RecordSessionBackground(platform, sessionID, background)
}

func (r *countingVisibilityRepo) writeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writes
}

func newVisibilityTestCache(t *testing.T) (*Cache, *sessionapp.Service, *countingVisibilityRepo) {
	t.Helper()
	repo, cleanup := newSessionRepositoryForTest(t)
	t.Cleanup(cleanup)
	counting := &countingVisibilityRepo{SQLiteRepository: repo}
	service := sessionapp.NewService(counting)
	return NewCache(service), service, counting
}

func visibleCodexSessions(t *testing.T, service *sessionapp.Service) map[string]bool {
	t.Helper()
	sessions, err := service.ListByPlatform("codex")
	if err != nil {
		t.Fatal(err)
	}
	visible := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		visible[session.SessionID] = true
	}
	return visible
}

func TestCacheRecordSessionBackgroundSkipsUnchangedWrites(t *testing.T) {
	cache, service, repo := newVisibilityTestCache(t)
	const gen = 0
	if err := cache.BindSessionToProviderGeneration("codex", "main", "alpha", gen); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if err := cache.RecordSessionBackground("codex", "main", false, gen); err != nil {
			t.Fatal(err)
		}
	}
	// 主会话上的后台请求（如记忆整理）不会改变可见性，也不应写库。
	if err := cache.RecordSessionBackground("codex", "main", true, gen); err != nil {
		t.Fatal(err)
	}
	if got := repo.writeCount(); got != 1 {
		t.Fatalf("expected a single visibility write for a stable main session, got %d", got)
	}
	if !visibleCodexSessions(t, service)["main"] {
		t.Fatal("main session must be visible")
	}
}

func TestCacheRecordSessionBackgroundPromotesBackgroundSession(t *testing.T) {
	cache, service, repo := newVisibilityTestCache(t)
	const gen = 0
	if err := cache.BindSessionToProviderGeneration("codex", "task", "alpha", gen); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := cache.RecordSessionBackground("codex", "task", true, gen); err != nil {
			t.Fatal(err)
		}
	}
	if got := repo.writeCount(); got != 1 {
		t.Fatalf("expected repeated background requests to write once, got %d", got)
	}
	if visibleCodexSessions(t, service)["task"] {
		t.Fatal("background-only session must stay hidden")
	}

	if err := cache.RecordSessionBackground("codex", "task", false, gen); err != nil {
		t.Fatal(err)
	}
	if got := repo.writeCount(); got != 2 {
		t.Fatalf("expected promotion to write, got %d writes", got)
	}
	if !visibleCodexSessions(t, service)["task"] {
		t.Fatal("promoted session must become visible")
	}
}

// 解绑后重新绑定会以"未分类"插入新行；缓存不能沿用旧的"已可见"而跳过写库，
// 否则重新绑定的会话会一直不出现在会话列表里。
func TestCacheRecordSessionBackgroundRewritesAfterRebind(t *testing.T) {
	cache, service, repo := newVisibilityTestCache(t)
	if err := cache.BindSessionToProviderGeneration("codex", "main", "alpha", 0); err != nil {
		t.Fatal(err)
	}
	if err := cache.RecordSessionBackground("codex", "main", false, 0); err != nil {
		t.Fatal(err)
	}

	if err := service.Unbind("codex", "main"); err != nil {
		t.Fatal(err)
	}
	generation := cache.SessionGeneration("codex", "main")
	if err := cache.BindSessionToProviderGeneration("codex", "main", "beta", generation); err != nil {
		t.Fatal(err)
	}
	if visibleCodexSessions(t, service)["main"] {
		t.Fatal("precondition: a fresh Codex binding starts unclassified")
	}
	if err := cache.RecordSessionBackground("codex", "main", false, generation); err != nil {
		t.Fatal(err)
	}
	if got := repo.writeCount(); got != 2 {
		t.Fatalf("expected visibility to be rewritten after rebind, got %d writes", got)
	}
	if !visibleCodexSessions(t, service)["main"] {
		t.Fatal("rebound main session must be visible again")
	}
}

func TestCacheRecordSessionBackgroundIgnoresStaleGeneration(t *testing.T) {
	cache, _, repo := newVisibilityTestCache(t)
	if err := cache.BindSessionToProviderGeneration("codex", "main", "alpha", 0); err != nil {
		t.Fatal(err)
	}
	cache.InvalidateSession("codex", "main")
	if err := cache.RecordSessionBackground("codex", "main", false, 0); err != nil {
		t.Fatal(err)
	}
	if got := repo.writeCount(); got != 0 {
		t.Fatalf("stale generation must not write, got %d", got)
	}
}
