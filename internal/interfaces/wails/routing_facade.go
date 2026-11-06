package wails

import (
	"context"

	routingapp "portico/internal/routing/application"
	routingdomain "portico/internal/routing/domain"
)

type RoutingFacade struct {
	service *routingapp.Service
}

func NewRoutingFacade(service *routingapp.Service) *RoutingFacade {
	return &RoutingFacade{service: service}
}

func (f *RoutingFacade) GetProfile(platform string) (routingdomain.RouteProfile, error) {
	return f.service.GetProfile(context.Background(), platform)
}

func (f *RoutingFacade) SaveProfile(profile routingdomain.RouteProfile) (routingdomain.RouteProfile, error) {
	return f.service.SaveProfile(context.Background(), profile)
}

func (f *RoutingFacade) SaveModelMappingScheme(
	platform string,
	providerID int,
	originalName string,
	scheme routingdomain.ModelMappingScheme,
	activate bool,
) (routingdomain.RouteProfile, error) {
	return f.service.SaveModelMappingScheme(context.Background(), platform, providerID, originalName, scheme, activate)
}

func (f *RoutingFacade) DeleteModelMappingScheme(platform string, providerID int, name string) (routingdomain.RouteProfile, error) {
	return f.service.DeleteModelMappingScheme(context.Background(), platform, providerID, name)
}

func (f *RoutingFacade) SetActiveModelMappingScheme(platform string, providerID int, name string) (routingdomain.RouteProfile, error) {
	return f.service.SetActiveModelMappingScheme(context.Background(), platform, providerID, name)
}

func (f *RoutingFacade) GetAppPreferences() (routingdomain.AppPreferences, error) {
	return f.service.GetAppPreferences(context.Background())
}

func (f *RoutingFacade) SaveAppPreferences(preferences routingdomain.AppPreferences) (routingdomain.AppPreferences, error) {
	return f.service.SaveAppPreferences(context.Background(), preferences)
}

// TestProvider 以供应商的代理/TLS 设置探测其 API Base URL 的连通性。
func (f *RoutingFacade) TestProvider(provider routingdomain.Provider) (routingapp.ProviderProbeResult, error) {
	return f.service.ProbeProvider(context.Background(), provider)
}
