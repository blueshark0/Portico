package wails

import (
	"context"

	configsyncapp "portico/internal/configsync/application"
	configsyncdomain "portico/internal/configsync/domain"
)

type ConfigSyncFacade struct {
	service *configsyncapp.Service
}

func NewConfigSyncFacade(service *configsyncapp.Service) *ConfigSyncFacade {
	return &ConfigSyncFacade{service: service}
}

func (f *ConfigSyncFacade) GetSyncSettings() (configsyncdomain.SyncSettings, error) {
	return f.service.GetSyncSettings(context.Background())
}

func (f *ConfigSyncFacade) SaveSyncSettings(settings configsyncdomain.SyncSettings) (configsyncdomain.SyncSettings, error) {
	return f.service.SaveSyncSettings(context.Background(), settings)
}

func (f *ConfigSyncFacade) TestConnection() error {
	return f.service.TestConnection(context.Background())
}

func (f *ConfigSyncFacade) GetRemoteSnapshotMeta() (configsyncdomain.RemoteSnapshotMeta, error) {
	return f.service.GetRemoteSnapshotMeta(context.Background())
}

func (f *ConfigSyncFacade) PushSnapshot(ui configsyncdomain.UIPreferences) (configsyncdomain.RemoteSnapshotMeta, error) {
	return f.service.PushSnapshot(context.Background(), ui)
}

func (f *ConfigSyncFacade) PullSnapshot() (configsyncdomain.ConfigSnapshot, error) {
	return f.service.PullSnapshot(context.Background())
}

func (f *ConfigSyncFacade) CreateLocalRollback(ui configsyncdomain.UIPreferences) (string, error) {
	return f.service.CreateLocalRollback(context.Background(), ui)
}

func (f *ConfigSyncFacade) ApplySnapshot(snapshot configsyncdomain.BackendSnapshot) error {
	return f.service.ApplySnapshot(context.Background(), snapshot)
}
