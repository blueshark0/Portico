package application

import (
	"path/filepath"
	"strings"
)

// stdioEnvironment keeps the host environment, but removes AppImage's Python
// paths: external Python processes must load their own standard library.
// Explicit per-server overrides are applied after cleaning inherited values.
func stdioEnvironment(inherited []string, overrides map[string]string) []string {
	var appDir string
	for _, entry := range inherited {
		if value, ok := strings.CutPrefix(entry, "APPDIR="); ok {
			appDir = value
		}
	}
	insideAppDir := func(path string) bool {
		if appDir == "" || !filepath.IsAbs(appDir) || !filepath.IsAbs(path) {
			return false
		}
		rel, err := filepath.Rel(appDir, path)
		return err == nil && filepath.IsLocal(rel)
	}

	env := make([]string, 0, len(inherited)+len(overrides))
	for _, entry := range inherited {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "PYTHONHOME":
			// Python also allows a prefix/exec_prefix pair.
			remove := false
			for _, path := range filepath.SplitList(value) {
				remove = remove || insideAppDir(path)
			}
			if remove {
				continue
			}
		case "PYTHONPATH":
			paths := filepath.SplitList(value)
			kept := make([]string, 0, len(paths))
			for _, path := range paths {
				if !insideAppDir(path) {
					kept = append(kept, path)
				}
			}
			if len(kept) != len(paths) {
				value = strings.Join(kept, string(filepath.ListSeparator))
				if value == "" {
					continue
				}
				entry = key + "=" + value
			}
		}
		env = append(env, entry)
	}
	return append(env, envPairs(overrides)...)
}
