package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"portico/internal/routing/domain"
	"portico/internal/shared/kernel"
	"portico/internal/shared/storage"
)

type LegacyImporter struct {
	store *SQLiteStore
	// appDir 为旧配置目录；为空时回退到 storage.AppDataDir()（默认行为）。
	appDir string
}

func NewLegacyImporter(store *SQLiteStore) *LegacyImporter {
	return &LegacyImporter{store: store}
}

// NewLegacyImporterWithDir 允许注入旧配置目录，便于测试。
func NewLegacyImporterWithDir(store *SQLiteStore, appDir string) *LegacyImporter {
	return &LegacyImporter{store: store, appDir: appDir}
}

func (i *LegacyImporter) EnsureImported(ctx context.Context) error {
	if i == nil || i.store == nil {
		return nil
	}
	imported, err := i.store.HasLegacyImport(ctx)
	if err != nil {
		return err
	}
	if imported {
		return nil
	}

	appDir, err := i.legacyAppDir()
	if err != nil {
		return err
	}
	legacySettings, err := loadLegacyAppSettings(filepath.Join(appDir, "app.json"))
	if err != nil {
		return err
	}
	if _, err := i.store.SaveAppPreferences(ctx, domain.AppPreferences{
		ShowHeatmap:   legacySettings.ShowHeatmap,
		ShowHomeTitle: legacySettings.ShowHomeTitle,
	}); err != nil {
		return err
	}

	if err := backupLegacyFiles(appDir); err != nil {
		return err
	}

	for _, platform := range kernel.AllPlatforms() {
		legacyProviders, exists, err := loadLegacyProviders(appDir, platform)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		profile := domain.RouteProfile{
			Platform:  platform,
			Providers: make([]domain.Provider, 0, len(legacyProviders)),
		}
		for idx, provider := range legacyProviders {
			position := provider.Position
			if position <= 0 {
				position = idx + 1
			}
			profile.Providers = append(profile.Providers, domain.Provider{
				ID:              provider.ID,
				Name:            provider.Name,
				APIURL:          provider.APIURL,
				APIKey:          provider.APIKey,
				OfficialSite:    provider.OfficialSite,
				Tint:            provider.Tint,
				Accent:          provider.Accent,
				Position:        position,
				SupportedModels: cloneBoolMap(provider.SupportedModels),
				ModelMapping:    cloneStringMap(provider.ModelMapping),
			})
		}
		defaultName := legacySettings.DefaultProviderName(platform)
		if defaultName != "" {
			if provider := profile.FindProviderByName(defaultName); provider != nil {
				defaultProviderID := provider.ID
				profile.DefaultProviderID = &defaultProviderID
			}
		}
		profile = profile.Normalize()
		if _, err := i.store.SaveProfile(ctx, profile); err != nil {
			return err
		}
	}

	return i.store.MarkLegacyImported(ctx)
}

type legacyProvider struct {
	ID              int               `json:"id"`
	Name            string            `json:"name"`
	APIURL          string            `json:"apiUrl"`
	APIKey          string            `json:"apiKey"`
	OfficialSite    string            `json:"officialSite"`
	Tint            string            `json:"tint"`
	Accent          string            `json:"accent"`
	Enabled         bool              `json:"enabled"`
	Position        int               `json:"position"`
	SupportedModels map[string]bool   `json:"supportedModels,omitempty"`
	ModelMapping    map[string]string `json:"modelMapping,omitempty"`
}

type legacyProviderEnvelope struct {
	Providers []legacyProvider `json:"providers"`
}

type legacyAppSettings struct {
	ShowHeatmap           bool   `json:"show_heatmap"`
	ShowHomeTitle         bool   `json:"show_home_title"`
	DefaultClaudeProvider string `json:"default_claude_provider"`
	DefaultCodexProvider  string `json:"default_codex_provider"`
	DefaultGeminiProvider string `json:"default_gemini_provider"`
}

func defaultLegacyAppSettings() legacyAppSettings {
	return legacyAppSettings{
		ShowHeatmap:   true,
		ShowHomeTitle: true,
	}
}

func (s legacyAppSettings) DefaultProviderName(platform kernel.Platform) string {
	switch platform {
	case kernel.PlatformClaude:
		return s.DefaultClaudeProvider
	case kernel.PlatformCodex:
		return s.DefaultCodexProvider
	case kernel.PlatformGemini:
		return s.DefaultGeminiProvider
	default:
		return ""
	}
}

func (i *LegacyImporter) legacyAppDir() (string, error) {
	if i.appDir != "" {
		return i.appDir, nil
	}
	return storage.AppDataDir()
}

func legacyProviderFile(appDir string, platform kernel.Platform) string {
	switch platform {
	case kernel.PlatformClaude:
		return filepath.Join(appDir, "claude-code.json")
	case kernel.PlatformCodex:
		return filepath.Join(appDir, "codex.json")
	case kernel.PlatformGemini:
		return filepath.Join(appDir, "gemini.json")
	default:
		return ""
	}
}

func loadLegacyProviders(appDir string, platform kernel.Platform) ([]legacyProvider, bool, error) {
	path := legacyProviderFile(appDir, platform)
	if path == "" {
		return nil, false, fmt.Errorf("unknown platform: %s", platform)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(data) == 0 {
		return []legacyProvider{}, true, nil
	}
	var envelope legacyProviderEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, false, err
	}
	return envelope.Providers, true, nil
}

func loadLegacyAppSettings(path string) (legacyAppSettings, error) {
	settings := defaultLegacyAppSettings()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		return settings, err
	}
	if len(data) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, err
	}
	return settings, nil
}

func backupLegacyFiles(appDir string) error {
	files := []string{
		filepath.Join(appDir, "claude-code.json"),
		filepath.Join(appDir, "codex.json"),
		filepath.Join(appDir, "gemini.json"),
		filepath.Join(appDir, "app.json"),
	}
	existing := make([]string, 0, len(files))
	for _, path := range files {
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, path)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	backupDir := filepath.Join(appDir, "legacy-backup", time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	for _, source := range existing {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		target := filepath.Join(backupDir, filepath.Base(source))
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	if len(source) == 0 {
		return map[string]bool{}
	}
	target := make(map[string]bool, len(source))
	for key, value := range source {
		target[strings.TrimSpace(key)] = value
	}
	return target
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	target := make(map[string]string, len(source))
	for key, value := range source {
		target[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return target
}
