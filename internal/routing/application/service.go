package application

import (
	"context"
	"fmt"
	"sync"

	"portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

type Repository interface {
	GetProfile(ctx context.Context, platform kernel.Platform) (domain.RouteProfile, error)
	SaveProfile(ctx context.Context, profile domain.RouteProfile) (domain.RouteProfile, error)
	GetAppPreferences(ctx context.Context) (domain.AppPreferences, error)
	SaveAppPreferences(ctx context.Context, preferences domain.AppPreferences) (domain.AppPreferences, error)
	// ReplaceAll 在单个事务里替换偏好与全部 profile（云端快照恢复用）。
	ReplaceAll(ctx context.Context, preferences domain.AppPreferences, profiles []domain.RouteProfile) error
}

// ProxyConfigSink 在偏好保存成功后接收最新代理配置（由 relay server 实现）。
// 通知必须发生在 application 层："偏好持久化 ⇒ 中继立即生效"这一不变式
// 对所有调用入口（UI facade、configsync 快照恢复）都要成立，不能只在
// 某个 facade 里维护。
type ProxyConfigSink interface {
	SetProxyConfig(enabled bool, url string)
}

type Service struct {
	repo Repository

	profileCacheMu sync.RWMutex
	profileCache   map[kernel.Platform]domain.RouteProfile

	proxySinkMu sync.RWMutex
	proxySink   ProxyConfigSink
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:         repo,
		profileCache: make(map[kernel.Platform]domain.RouteProfile),
	}
}

func (s *Service) GetProfile(ctx context.Context, platform string) (domain.RouteProfile, error) {
	parsed, err := kernel.ParsePlatform(platform)
	if err != nil {
		return domain.RouteProfile{}, err
	}
	return s.getProfile(ctx, parsed)
}

func (s *Service) SaveProfile(ctx context.Context, profile domain.RouteProfile) (domain.RouteProfile, error) {
	return s.saveProfile(ctx, profile, true)
}

func (s *Service) ReplaceProfile(ctx context.Context, profile domain.RouteProfile) (domain.RouteProfile, error) {
	return s.saveProfile(ctx, profile, false)
}

func (s *Service) saveProfile(ctx context.Context, profile domain.RouteProfile, enforceStableNames bool) (domain.RouteProfile, error) {
	profile = profile.Normalize()
	if err := profile.Validate(); err != nil {
		return domain.RouteProfile{}, err
	}
	if enforceStableNames {
		current, err := s.getProfile(ctx, profile.Platform)
		if err != nil {
			return domain.RouteProfile{}, err
		}
		originalNames := make(map[int]string, len(current.Providers))
		for _, provider := range current.Providers {
			originalNames[provider.ID] = provider.Name
		}
		for _, provider := range profile.Providers {
			if oldName, ok := originalNames[provider.ID]; ok && oldName != provider.Name {
				return domain.RouteProfile{}, fmt.Errorf("provider id %d 的 name 不可修改", provider.ID)
			}
		}
	}
	saved, err := s.repo.SaveProfile(ctx, profile)
	if err != nil {
		return domain.RouteProfile{}, err
	}
	s.storeProfileCache(saved)
	return saved, nil
}

func (s *Service) GetAppPreferences(ctx context.Context) (domain.AppPreferences, error) {
	return s.repo.GetAppPreferences(ctx)
}

func (s *Service) SaveAppPreferences(ctx context.Context, preferences domain.AppPreferences) (domain.AppPreferences, error) {
	saved, err := s.repo.SaveAppPreferences(ctx, preferences)
	if err != nil {
		return domain.AppPreferences{}, err
	}
	s.notifyProxyConfig(saved)
	return saved, nil
}

// ReplaceAll 原子替换应用偏好与全部平台 profile（云端快照恢复入口）：
// 全部校验通过后单事务落库，避免恢复到一半失败留下混合状态；
// 成功后刷新缓存并推送代理配置到 relay。
func (s *Service) ReplaceAll(ctx context.Context, preferences domain.AppPreferences, profiles ...domain.RouteProfile) error {
	normalized := make([]domain.RouteProfile, 0, len(profiles))
	for _, profile := range profiles {
		next := profile.Normalize()
		if err := next.Validate(); err != nil {
			return err
		}
		normalized = append(normalized, next)
	}
	if err := s.repo.ReplaceAll(ctx, preferences, normalized); err != nil {
		return err
	}
	for _, profile := range normalized {
		s.storeProfileCache(profile)
	}
	s.notifyProxyConfig(preferences)
	return nil
}

// RegisterProxyConfigSink 由组合根在装配期调用（relay server 构造依赖本
// Service，无法在构造函数里互相注入）。
func (s *Service) RegisterProxyConfigSink(sink ProxyConfigSink) {
	s.proxySinkMu.Lock()
	s.proxySink = sink
	s.proxySinkMu.Unlock()
}

func (s *Service) notifyProxyConfig(preferences domain.AppPreferences) {
	s.proxySinkMu.RLock()
	sink := s.proxySink
	s.proxySinkMu.RUnlock()
	if sink != nil {
		sink.SetProxyConfig(preferences.ProxyEnabled, preferences.ProxyURL)
	}
}

func (s *Service) getProfile(ctx context.Context, platform kernel.Platform) (domain.RouteProfile, error) {
	if cached, ok := s.loadProfileCache(platform); ok {
		return cached, nil
	}
	profile, err := s.repo.GetProfile(ctx, platform)
	if err != nil {
		return domain.RouteProfile{}, err
	}
	s.storeProfileCache(profile)
	return cloneRouteProfile(profile), nil
}

func (s *Service) loadProfileCache(platform kernel.Platform) (domain.RouteProfile, bool) {
	s.profileCacheMu.RLock()
	defer s.profileCacheMu.RUnlock()

	profile, ok := s.profileCache[platform]
	if !ok {
		return domain.RouteProfile{}, false
	}
	return cloneRouteProfile(profile), true
}

func (s *Service) storeProfileCache(profile domain.RouteProfile) {
	s.profileCacheMu.Lock()
	defer s.profileCacheMu.Unlock()
	s.profileCache[profile.Platform] = cloneRouteProfile(profile.Normalize())
}

func cloneRouteProfile(profile domain.RouteProfile) domain.RouteProfile {
	cloned := domain.RouteProfile{
		Platform:  profile.Platform,
		Providers: make([]domain.Provider, 0, len(profile.Providers)),
	}
	if profile.DefaultProviderID != nil {
		value := *profile.DefaultProviderID
		cloned.DefaultProviderID = &value
	}
	for _, provider := range profile.Providers {
		next := provider
		if provider.ArchivedAt != nil {
			archivedAt := *provider.ArchivedAt
			next.ArchivedAt = &archivedAt
		}
		next.SupportedModels = cloneBoolMap(provider.SupportedModels)
		next.ModelMapping = cloneStringMap(provider.ModelMapping)
		next.RequestBodyReplacements = cloneRequestBodyReplacements(provider.RequestBodyReplacements)
		cloned.Providers = append(cloned.Providers, next)
	}
	return cloned
}

func cloneBoolMap(input map[string]bool) map[string]bool {
	if input == nil {
		return map[string]bool{}
	}
	cloned := make(map[string]bool, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneRequestBodyReplacements(input []domain.RequestBodyReplacement) []domain.RequestBodyReplacement {
	if input == nil {
		return []domain.RequestBodyReplacement{}
	}
	return append([]domain.RequestBodyReplacement(nil), input...)
}
