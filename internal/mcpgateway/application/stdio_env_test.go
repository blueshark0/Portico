package application

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStdioEnvironment(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "app")
	hostPython := filepath.Join(t.TempDir(), "python")
	appPython := filepath.Join(appDir, "usr")
	appModules := filepath.Join(appPython, "share", "pyshared")
	separator := string(os.PathListSeparator)
	for _, tt := range []struct {
		name      string
		inherited []string
		overrides map[string]string
		want      map[string]string
	}{
		{
			name: "AppImage Python paths are removed",
			inherited: []string{"APPDIR=" + appDir, "PYTHONHOME=" + appPython,
				"PYTHONPATH=" + appModules + separator, "PATH=host-path", "HOME=host-home"},
			want: map[string]string{"APPDIR": appDir, "PATH": "host-path", "HOME": "host-home"},
		},
		{
			name: "host Python paths and neighboring directories survive",
			inherited: []string{"APPDIR=" + appDir, "PYTHONHOME=" + hostPython,
				"PYTHONPATH=" + strings.Join([]string{hostPython, appModules, appDir + "-other"}, separator)},
			want: map[string]string{"APPDIR": appDir, "PYTHONHOME": hostPython,
				"PYTHONPATH": hostPython + separator + appDir + "-other"},
		},
		{
			name:      "ordinary launch is unchanged",
			inherited: []string{"PYTHONHOME=" + hostPython, "PYTHONPATH=" + appModules},
			want:      map[string]string{"PYTHONHOME": hostPython, "PYTHONPATH": appModules},
		},
		{
			name: "explicit server overrides win",
			inherited: []string{"APPDIR=" + appDir, "PYTHONHOME=" + appPython,
				"PYTHONPATH=" + appModules, "TOKEN=inherited"},
			overrides: map[string]string{"PYTHONHOME": appPython, "PYTHONPATH": appModules, "TOKEN": "configured"},
			want: map[string]string{"APPDIR": appDir, "PYTHONHOME": appPython,
				"PYTHONPATH": appModules, "TOKEN": "configured"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := append([]string(nil), tt.inherited...)
			got := make(map[string]string)
			for _, entry := range stdioEnvironment(tt.inherited, tt.overrides) {
				key, value, _ := strings.Cut(entry, "=")
				got[key] = value // os/exec uses the last value for duplicate keys.
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("environment = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.inherited, before) {
				t.Fatal("modified inherited environment")
			}
		})
	}
}
