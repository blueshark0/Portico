package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"portico/internal/shared/serverinfo"
)

// fakeBackend 模拟 portico-server 的 health / session / shutdown 接口。
type fakeBackend struct {
	version      atomic.Value // string
	requireToken string
	up           atomic.Bool
	shutdowns    atomic.Int32
	lastAuth     atomic.Value
}

func newFakeBackend(version, requireToken string) *fakeBackend {
	f := &fakeBackend{requireToken: requireToken}
	f.version.Store(version)
	return f
}

func (f *fakeBackend) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if !f.up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		version, _ := f.version.Load().(string)
		_ = json.NewEncoder(w).Encode(serverinfo.Health{Status: "ok", Version: version, PID: 1})
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		if f.requireToken != "" && r.Header.Get("Authorization") != "Bearer "+f.requireToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		f.lastAuth.Store(r.Header.Get("Authorization"))
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.shutdowns.Add(1)
		f.up.Store(false)
		w.WriteHeader(http.StatusAccepted)
	})
	return mux
}

type testEnv struct {
	dir         string
	runtimePath string
	tokenPath   string
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	dir := t.TempDir()
	env := testEnv{
		dir:         dir,
		runtimePath: filepath.Join(dir, "server.json"),
		tokenPath:   filepath.Join(dir, "admin_token"),
	}
	if err := os.WriteFile(env.tokenPath, []byte("tok-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return env
}

func (e testEnv) writeRuntime(t *testing.T, adminURL string) {
	t.Helper()
	if err := serverinfo.WriteRuntimeInfo(e.runtimePath, serverinfo.RuntimeInfo{PID: 1, AdminURL: adminURL}); err != nil {
		t.Fatal(err)
	}
}

type spawnRecorder struct {
	mu      sync.Mutex
	calls   []string
	onSpawn func()
	err     error
	exited  chan error
}

func (s *spawnRecorder) spawn(binary string) (<-chan error, error) {
	s.mu.Lock()
	s.calls = append(s.calls, binary)
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	if s.onSpawn != nil {
		go s.onSpawn()
	}
	if s.exited == nil {
		s.exited = make(chan error, 1)
	}
	return s.exited, nil
}

func (s *spawnRecorder) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func newManager(env testEnv, defaultURL string, spawner *spawnRecorder, version string) *Manager {
	return New(Options{
		Version:         version,
		ServerBinary:    "/opt/portico/portico-server",
		RuntimeInfoPath: env.runtimePath,
		TokenPath:       env.tokenPath,
		DefaultURL:      defaultURL,
		StartTimeout:    2 * time.Second,
		Spawn:           spawner.spawn,
	})
}

func TestEnsureReusesRunningBackendWithSameVersion(t *testing.T) {
	env := newTestEnv(t)
	backend := newFakeBackend("v1", "")
	backend.up.Store(true)
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()
	env.writeRuntime(t, srv.URL)

	spawner := &spawnRecorder{}
	conn, err := newManager(env, "http://127.0.0.1:1", spawner, "v1").Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if conn.AdminURL != srv.URL || conn.Spawned {
		t.Fatalf("conn = %+v", conn)
	}
	if spawner.count() != 0 {
		t.Fatal("must not spawn when a compatible backend is running")
	}
}

func TestEnsureSpawnsWhenNothingIsRunning(t *testing.T) {
	env := newTestEnv(t)
	backend := newFakeBackend("v1", "")
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	spawner := &spawnRecorder{onSpawn: func() {
		time.Sleep(300 * time.Millisecond)
		backend.up.Store(true)
	}}
	conn, err := newManager(env, srv.URL, spawner, "v1").Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !conn.Spawned || conn.AdminURL != srv.URL {
		t.Fatalf("conn = %+v", conn)
	}
	if spawner.count() != 1 || spawner.calls[0] != "/opt/portico/portico-server" {
		t.Fatalf("spawn calls = %v", spawner.calls)
	}
}

func TestEnsureRestartsBackendOnVersionMismatch(t *testing.T) {
	env := newTestEnv(t)
	backend := newFakeBackend("v0", "")
	backend.up.Store(true)
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()
	env.writeRuntime(t, srv.URL)

	spawner := &spawnRecorder{onSpawn: func() {
		backend.version.Store("v1")
		backend.up.Store(true)
	}}
	conn, err := newManager(env, srv.URL, spawner, "v1").Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if backend.shutdowns.Load() != 1 {
		t.Fatalf("shutdowns = %d, want 1", backend.shutdowns.Load())
	}
	if got, _ := backend.lastAuth.Load().(string); got != "Bearer tok-123" {
		t.Fatalf("shutdown should carry the admin token, got %q", got)
	}
	if !conn.Spawned || spawner.count() != 1 {
		t.Fatalf("conn = %+v, spawns = %d", conn, spawner.count())
	}
}

func TestEnsureReportsEarlyExit(t *testing.T) {
	env := newTestEnv(t)
	backend := newFakeBackend("v1", "")
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	exited := make(chan error, 1)
	exited <- errors.New("exit status 1")
	spawner := &spawnRecorder{exited: exited}
	_, err := newManager(env, srv.URL, spawner, "v1").Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("want early-exit error, got %v", err)
	}
}

func TestEnsureTimesOut(t *testing.T) {
	env := newTestEnv(t)
	backend := newFakeBackend("v1", "")
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	m := newManager(env, srv.URL, &spawnRecorder{}, "v1")
	m.opts.StartTimeout = 500 * time.Millisecond
	start := time.Now()
	_, err := m.Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("want timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestEnsureSpawnError(t *testing.T) {
	env := newTestEnv(t)
	spawner := &spawnRecorder{err: errors.New("permission denied")}
	_, err := newManager(env, "http://127.0.0.1:1", spawner, "v1").Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want spawn error, got %v", err)
	}
}

func TestServerBinaryMissing(t *testing.T) {
	m := New(Options{})
	if _, err := m.serverBinary(); err == nil {
		// 测试二进制旁边不会有 portico-server。
		t.Fatal("want error when portico-server is not next to the executable")
	}
}

func TestWindowURLAddsTokenOnlyWhenRequired(t *testing.T) {
	env := newTestEnv(t)

	open := newFakeBackend("v1", "")
	openSrv := httptest.NewServer(open.handler())
	defer openSrv.Close()
	m := newManager(env, openSrv.URL, &spawnRecorder{}, "v1")
	if got := m.WindowURL(context.Background(), Connection{AdminURL: openSrv.URL}); got != openSrv.URL+"/" {
		t.Fatalf("WindowURL = %q, want no token", got)
	}

	locked := newFakeBackend("v1", "tok-123")
	lockedSrv := httptest.NewServer(locked.handler())
	defer lockedSrv.Close()
	if got := m.WindowURL(context.Background(), Connection{AdminURL: lockedSrv.URL + "/"}); got != lockedSrv.URL+"/?token=tok-123" {
		t.Fatalf("WindowURL = %q, want token appended", got)
	}
}
