package wails

import (
	platformproxyapp "portico/internal/platformproxy/application"
	platformproxydomain "portico/internal/platformproxy/domain"
)

type PlatformProxyFacade struct {
	service *platformproxyapp.Service
}

func NewPlatformProxyFacade(service *platformproxyapp.Service) *PlatformProxyFacade {
	return &PlatformProxyFacade{service: service}
}

func (f *PlatformProxyFacade) GetStatus(platform string) (platformproxydomain.Status, error) {
	return f.service.GetStatus(platform)
}

func (f *PlatformProxyFacade) Enable(platform string) error {
	return f.service.Enable(platform)
}

func (f *PlatformProxyFacade) Disable(platform string) error {
	return f.service.Disable(platform)
}
