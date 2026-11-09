//go:build !windows

package sidecar

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestServerBinaryCopiesOutOfAppImage(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "mount")
	bundled := filepath.Join(appDir, "usr", "bin", "portico-server")
	if err := os.MkdirAll(filepath.Dir(bundled), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundled, []byte("new"), 0o555); err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(installDir, "portico-server")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Options{ServerBinary: bundled, AppImageDir: appDir, ServerInstallDir: installDir})

	got, err := m.serverBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("server path = %s, want %s", got, target)
	}
	content, err := os.ReadFile(got)
	if err != nil || string(content) != "new" {
		t.Fatalf("server content = %q, err %v", content, err)
	}
	info, err := os.Stat(got)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("server mode = %v, err %v", info, err)
	}
	entries, err := os.ReadDir(installDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, err %v", entries, err)
	}
}

func TestServerBinaryOutsideAppImageIsUsedInPlace(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "portico-server")
	installDir := t.TempDir()
	m := New(Options{ServerBinary: binary, AppImageDir: "/tmp/.mount_other", ServerInstallDir: installDir})
	got, err := m.serverBinary()
	if err != nil || got != binary {
		t.Fatalf("server path = %s, err %v", got, err)
	}
	if entries, _ := os.ReadDir(installDir); len(entries) != 0 {
		t.Fatalf("server should not be copied: %v", entries)
	}
}

func TestCleanAppImageEnv(t *testing.T) {
	const appDir = "/tmp/.mount_abc"
	got := cleanAppImageEnv([]string{
		"HOME=/home/me",
		"APPDIR=" + appDir,
		"APPIMAGE=/home/me/Portico.AppImage",
		"ARGV0=Portico",
		"OWD=/home/me",
		"PWD=" + appDir + "/usr/lib",
		"OLDPWD=/home/me",
		"GSETTINGS_SCHEMA_DIR=" + appDir + "/usr/share/glib-2.0/schemas",
		"WEBKIT_INJECTED_BUNDLE_PATH=" + appDir + "/usr/lib/webkit/",
		"PATH=" + appDir + "/usr/bin:/usr/local/bin:/usr/bin",
		"LD_LIBRARY_PATH=" + appDir + "/usr/lib:" + appDir + "/usr/lib64",
		"XDG_DATA_DIRS=" + appDir + "/usr/share:/usr/share",
		"KEEP=" + appDir + "/not-a-path-list",
		"OTHER_PATH=/tmp/.mount_abcd/usr/bin",
	}, appDir)
	want := []string{
		"HOME=/home/me",
		"PATH=/usr/local/bin:/usr/bin",
		"XDG_DATA_DIRS=/usr/share",
		"KEEP=" + appDir + "/not-a-path-list",
		"OTHER_PATH=/tmp/.mount_abcd/usr/bin",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("env:\n got %v\nwant %v", got, want)
	}
}

// 用脚本代替后端，确认真正拉起的进程没有继承 AppImage 的环境和工作目录。
func TestSpawnDetachedLeavesAppImageBehind(t *testing.T) {
	const appDir = "/tmp/.mount_test"
	t.Setenv("APPDIR", appDir)
	t.Setenv("LD_LIBRARY_PATH", appDir+"/usr/lib")
	t.Setenv("PWD", appDir+"/usr/lib")
	t.Setenv("PATH", appDir+"/usr/bin:"+os.Getenv("PATH"))
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	script := filepath.Join(dir, "portico-server")
	body := "#!/bin/sh\n{ pwd; env; } > \"$(dirname \"$0\")/out.tmp\"; mv \"$(dirname \"$0\")/out.tmp\" \"$(dirname \"$0\")/out\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	exited, err := spawnDetached(script)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("process did not exit")
	}
	out, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(out), "\n")
	if lines[0] != dir {
		t.Fatalf("cwd = %s, want %s", lines[0], dir)
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "APPDIR=") || strings.HasPrefix(line, "LD_LIBRARY_PATH=") || strings.Contains(line, appDir) {
			t.Fatalf("AppImage environment leaked: %s", line)
		}
	}
}
