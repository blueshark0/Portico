package relay

import (
	"context"
	"fmt"

	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func (s *Server) validateConfig() []string {
	warnings := make([]string, 0)
	for _, platform := range kernel.AllPlatforms() {
		profile, err := s.routingService.GetProfile(context.Background(), platform.String())
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("[%s] 加载配置失败: %v", platform, err))
			continue
		}
		providerCount := 0
		for _, p := range profile.Providers {
			providerCount++
			if errs := p.ValidateConfiguration(); len(errs) > 0 {
				for _, errMsg := range errs {
					warnings = append(warnings, fmt.Sprintf("[%s/%s] %s", platform, p.Name, errMsg))
				}
			}
		}
		if providerCount == 0 {
			warnings = append(warnings, fmt.Sprintf("[%s] 没有配置 provider", platform))
		}
	}
	return warnings
}

func (s *Server) loadProfile(kind string) (routingdomain.RouteProfile, error) {
	profile, err := s.routingService.GetProfile(context.Background(), kind)
	if err != nil {
		return routingdomain.RouteProfile{}, fmt.Errorf("failed to load providers: %w", err)
	}
	return profile, nil
}

func (s *Server) initProxyConfig() {
	prefs, err := s.routingService.GetAppPreferences(context.Background())
	if err != nil {
		relayWarnf("failed to load proxy preferences: %v", err)
		return
	}
	s.proxyMu.Lock()
	s.proxyEnabled = prefs.ProxyEnabled
	s.proxyURL = prefs.ProxyURL
	s.proxyMu.Unlock()
}

func (s *Server) SetProxyConfig(enabled bool, url string) {
	s.proxyMu.Lock()
	s.proxyEnabled = enabled
	s.proxyURL = url
	s.proxyMu.Unlock()
	s.resetUpstreamClients()
}

func (s *Server) getProxyURL() string {
	s.proxyMu.RLock()
	defer s.proxyMu.RUnlock()
	if s.proxyEnabled && s.proxyURL != "" {
		return s.proxyURL
	}
	return ""
}

func (s *Server) getProxyURLForProvider(provider *routingdomain.Provider) string {
	s.proxyMu.RLock()
	enabled := s.proxyEnabled
	globalURL := s.proxyURL
	s.proxyMu.RUnlock()
	proxyURL := provider.ResolveProxyURL(enabled, globalURL)
	if proxyURL != "" {
		relayDebugf("provider %s proxy: %s", provider.Name, proxyURL)
	}
	return proxyURL
}
