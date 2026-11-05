package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	configsyncdomain "portico/internal/configsync/domain"
	routingapp "portico/internal/routing/application"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
	"portico/internal/shared/storage"
)

type Repository interface {
	GetSettings(ctx context.Context) (configsyncdomain.SyncSettings, error)
	SaveSettings(ctx context.Context, settings configsyncdomain.SyncSettings) (configsyncdomain.SyncSettings, error)
}

type RemoteClient interface {
	TestConnection(ctx context.Context, settings configsyncdomain.SyncSettings) error
	Upload(ctx context.Context, settings configsyncdomain.SyncSettings, filename string, payload []byte) error
	Download(ctx context.Context, settings configsyncdomain.SyncSettings, filename string) ([]byte, error)
}

type Service struct {
	repo        Repository
	remote      RemoteClient
	routing     *routingapp.Service
	rollbackDir string
	appVersion  string
	now         func() time.Time
	hostname    func() (string, error)
}

func NewService(
	repo Repository,
	remote RemoteClient,
	routing *routingapp.Service,
	appVersion string,
) *Service {
	rollbackDir, _ := storage.AppDataPath(configsyncdomain.LocalRollbackDirName)
	return &Service{
		repo:        repo,
		remote:      remote,
		routing:     routing,
		rollbackDir: rollbackDir,
		appVersion:  appVersion,
		now:         time.Now,
		hostname:    os.Hostname,
	}
}

func (s *Service) GetSyncSettings(ctx context.Context) (configsyncdomain.SyncSettings, error) {
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return configsyncdomain.SyncSettings{}, err
	}
	return normalizeSettings(settings), nil
}

func (s *Service) SaveSyncSettings(ctx context.Context, settings configsyncdomain.SyncSettings) (configsyncdomain.SyncSettings, error) {
	return s.repo.SaveSettings(ctx, normalizeSettings(settings))
}

func (s *Service) TestConnection(ctx context.Context) error {
	settings, err := s.GetSyncSettings(ctx)
	if err != nil {
		return err
	}
	if err := validateConnectionSettings(settings); err != nil {
		return err
	}
	return s.remote.TestConnection(ctx, settings)
}

func (s *Service) GetRemoteSnapshotMeta(ctx context.Context) (configsyncdomain.RemoteSnapshotMeta, error) {
	settings, err := s.GetSyncSettings(ctx)
	if err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}
	if err := validateConnectionSettings(settings); err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}

	payload, err := s.downloadRemoteSnapshot(ctx, settings)
	if err != nil {
		if errors.Is(err, ErrRemoteSnapshotNotFound) {
			return configsyncdomain.RemoteSnapshotMeta{}, nil
		}
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}
	return ReadEnvelopeMeta(payload)
}

func (s *Service) PushSnapshot(ctx context.Context, ui configsyncdomain.UIPreferences) (configsyncdomain.RemoteSnapshotMeta, error) {
	settings, err := s.GetSyncSettings(ctx)
	if err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}
	if err := validateRunnableSettings(settings); err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}

	snapshot, err := s.buildSnapshot(ctx, normalizeUI(ui))
	if err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}

	deviceName := s.deviceName()
	payload, meta, err := EncryptSnapshot(snapshot, settings.SyncPassphrase, deviceName, s.appVersion, s.now())
	if err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}
	if err := s.remote.Upload(ctx, settings, configsyncdomain.RemoteSnapshotFile, payload); err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}
	return meta, nil
}

func (s *Service) downloadRemoteSnapshot(ctx context.Context, settings configsyncdomain.SyncSettings) ([]byte, error) {
	payload, err := s.remote.Download(ctx, settings, configsyncdomain.RemoteSnapshotFile)
	if !errors.Is(err, ErrRemoteSnapshotNotFound) {
		return payload, err
	}
	return s.remote.Download(ctx, settings, configsyncdomain.LegacyRemoteSnapshotFile)
}

func (s *Service) PullSnapshot(ctx context.Context) (configsyncdomain.ConfigSnapshot, error) {
	settings, err := s.GetSyncSettings(ctx)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}
	if err := validateRunnableSettings(settings); err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}

	payload, err := s.downloadRemoteSnapshot(ctx, settings)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}

	snapshot, _, err := DecryptSnapshot(payload, settings.SyncPassphrase)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}
	return validateSnapshot(snapshot)
}

func (s *Service) CreateLocalRollback(ctx context.Context, ui configsyncdomain.UIPreferences) (string, error) {
	settings, err := s.GetSyncSettings(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(settings.SyncPassphrase) == "" {
		return "", fmt.Errorf("请先保存同步口令")
	}

	snapshot, err := s.buildSnapshot(ctx, normalizeUI(ui))
	if err != nil {
		return "", err
	}
	payload, _, err := EncryptSnapshot(snapshot, settings.SyncPassphrase, s.deviceName(), s.appVersion, s.now())
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(s.rollbackDir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("rollback-%s.enc.json", s.now().Format("20060102-150405"))
	target := filepath.Join(s.rollbackDir, filename)
	if err := os.WriteFile(target, payload, 0o600); err != nil {
		return "", err
	}
	return target, nil
}

func (s *Service) ApplySnapshot(ctx context.Context, snapshot configsyncdomain.BackendSnapshot) error {
	snapshot = normalizeBackendSnapshot(snapshot)
	if err := validateBackendSnapshot(snapshot); err != nil {
		return err
	}
	// 单事务替换：任一平台失败都不能留下半应用的混合配置。
	return s.routing.ReplaceAll(ctx, snapshot.AppPreferences,
		snapshot.ClaudeProfile, snapshot.CodexProfile, snapshot.GeminiProfile)
}

func (s *Service) buildSnapshot(ctx context.Context, ui configsyncdomain.UIPreferences) (configsyncdomain.ConfigSnapshot, error) {
	preferences, err := s.routing.GetAppPreferences(ctx)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}
	claudeProfile, err := s.routing.GetProfile(ctx, kernel.PlatformClaude.String())
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}
	codexProfile, err := s.routing.GetProfile(ctx, kernel.PlatformCodex.String())
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}
	geminiProfile, err := s.routing.GetProfile(ctx, kernel.PlatformGemini.String())
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}

	return configsyncdomain.ConfigSnapshot{
		Backend: configsyncdomain.BackendSnapshot{
			AppPreferences: preferences,
			ClaudeProfile:  claudeProfile,
			CodexProfile:   codexProfile,
			GeminiProfile:  geminiProfile,
		},
		UI: ui,
	}, nil
}

func normalizeSettings(settings configsyncdomain.SyncSettings) configsyncdomain.SyncSettings {
	defaults := configsyncdomain.DefaultSyncSettings()
	settings.Endpoint = normalizeEndpoint(settings.Endpoint)
	settings.Username = strings.TrimSpace(settings.Username)
	settings.RemotePath = normalizeRemotePath(settings.RemotePath)
	if settings.RemotePath == "" {
		settings.RemotePath = defaults.RemotePath
	}
	return settings
}

func normalizeEndpoint(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}
	return strings.TrimRight(trimmed, "/")
}

func normalizeRemotePath(remotePath string) string {
	trimmed := strings.TrimSpace(remotePath)
	if trimmed == "" {
		return configsyncdomain.DefaultRemotePath
	}
	cleaned := filepath.ToSlash(trimmed)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	cleaned = filepath.ToSlash(filepath.Clean(cleaned))
	if cleaned == "." {
		return configsyncdomain.DefaultRemotePath
	}
	return cleaned
}

func normalizeUI(ui configsyncdomain.UIPreferences) configsyncdomain.UIPreferences {
	switch ui.Theme {
	case "light", "dark", "systemdefault":
	default:
		ui.Theme = "systemdefault"
	}
	switch ui.Locale {
	case "en", "zh":
	default:
		ui.Locale = "zh"
	}
	return ui
}

func normalizeSnapshot(snapshot configsyncdomain.ConfigSnapshot) configsyncdomain.ConfigSnapshot {
	snapshot.UI = normalizeUI(snapshot.UI)
	snapshot.Backend = normalizeBackendSnapshot(snapshot.Backend)
	return snapshot
}

func normalizeBackendSnapshot(snapshot configsyncdomain.BackendSnapshot) configsyncdomain.BackendSnapshot {
	snapshot.ClaudeProfile = snapshot.ClaudeProfile.Normalize()
	snapshot.CodexProfile = snapshot.CodexProfile.Normalize()
	snapshot.GeminiProfile = snapshot.GeminiProfile.Normalize()
	return snapshot
}

func validateRunnableSettings(settings configsyncdomain.SyncSettings) error {
	if err := validateConnectionSettings(settings); err != nil {
		return err
	}
	if strings.TrimSpace(settings.SyncPassphrase) == "" {
		return fmt.Errorf("请先保存同步口令")
	}
	return nil
}

func validateConnectionSettings(settings configsyncdomain.SyncSettings) error {
	endpoint := strings.TrimSpace(settings.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("请先填写 WebDAV 服务器地址")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return fmt.Errorf("WebDAV 服务器地址必须以 http:// 或 https:// 开头")
	}
	if strings.TrimSpace(settings.Username) == "" {
		return fmt.Errorf("请先填写 WebDAV 用户名")
	}
	if strings.TrimSpace(settings.AppPassword) == "" {
		return fmt.Errorf("请先填写 WebDAV 密码")
	}
	return nil
}

func validateSnapshot(snapshot configsyncdomain.ConfigSnapshot) (configsyncdomain.ConfigSnapshot, error) {
	snapshot = normalizeSnapshot(snapshot)
	if err := validateBackendSnapshot(snapshot.Backend); err != nil {
		return configsyncdomain.ConfigSnapshot{}, err
	}
	return snapshot, nil
}

func validateBackendSnapshot(snapshot configsyncdomain.BackendSnapshot) error {
	expected := map[kernel.Platform]routingdomain.RouteProfile{
		kernel.PlatformClaude: snapshot.ClaudeProfile,
		kernel.PlatformCodex:  snapshot.CodexProfile,
		kernel.PlatformGemini: snapshot.GeminiProfile,
	}
	for platform, profile := range expected {
		if profile.Platform != platform {
			return fmt.Errorf("云端配置缺少 %s 平台数据", platform.String())
		}
	}
	return nil
}

func (s *Service) deviceName() string {
	name, err := s.hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "unknown-device"
	}
	return strings.TrimSpace(name)
}
