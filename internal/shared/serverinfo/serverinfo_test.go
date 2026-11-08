package serverinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadOrCreateTokenPersistsAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "admin_token")

	first, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if len(first) < 40 {
		t.Fatalf("token too short: %q", first)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("token file perm = %o, want 600", perm)
		}
	}

	again, err := LoadOrCreateToken(path)
	if err != nil || again != first {
		t.Fatalf("second load = %q, %v; want %q", again, err, first)
	}

	reset, err := ResetToken(path)
	if err != nil || reset == first {
		t.Fatalf("reset = %q, %v", reset, err)
	}
	if loaded, _ := LoadOrCreateToken(path); loaded != reset {
		t.Fatalf("load after reset = %q, want %q", loaded, reset)
	}
}

func TestLoadOrCreateTokenRegeneratesEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin_token")
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := LoadOrCreateToken(path)
	if err != nil || token == "" {
		t.Fatalf("token = %q, %v", token, err)
	}
}

func TestRuntimeInfoRoundTripAndOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	info := RuntimeInfo{PID: 4242, AdminAddr: "127.0.0.1:18101", AdminURL: "http://127.0.0.1:18101", Version: "v1"}
	if err := WriteRuntimeInfo(path, info); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRuntimeInfo(path)
	if err != nil || got != info {
		t.Fatalf("read = %#v, %v", got, err)
	}

	RemoveRuntimeInfo(path, 1)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("file owned by another pid must not be removed")
	}
	RemoveRuntimeInfo(path, 4242)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, stat err = %v", err)
	}
}

func TestAdminURL(t *testing.T) {
	cases := []struct {
		addr string
		tls  bool
		want string
	}{
		{"127.0.0.1:18101", false, "http://127.0.0.1:18101"},
		{"0.0.0.0:18101", false, "http://127.0.0.1:18101"},
		{"[::]:18101", true, "https://127.0.0.1:18101"},
		{":18101", false, "http://127.0.0.1:18101"},
		{"portico.example:443", true, "https://portico.example:443"},
	}
	for _, tc := range cases {
		if got := AdminURL(tc.addr, tc.tls); got != tc.want {
			t.Errorf("AdminURL(%q, %v) = %q, want %q", tc.addr, tc.tls, got, tc.want)
		}
	}
}

func TestIsLoopbackListen(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:18101": true,
		"[::1]:18101":     true,
		"localhost:18101": true,
		"0.0.0.0:18101":   false,
		":18101":          false,
		"10.0.0.5:18101":  false,
		"garbage":         false,
	}
	for addr, want := range cases {
		if got := IsLoopbackListen(addr); got != want {
			t.Errorf("IsLoopbackListen(%q) = %v, want %v", addr, got, want)
		}
	}
}
