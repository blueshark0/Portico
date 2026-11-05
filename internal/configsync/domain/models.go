package domain

import (
	routingdomain "portico/internal/routing/domain"
)

const (
	DefaultRemotePath        = "/Portico"
	RemoteSnapshotFile       = "portico.config.v1.enc.json"
	LegacyRemoteSnapshotFile = "codeswitch.config.v1.enc.json"
	LocalRollbackDirName     = "sync-backups"
	SchemaVersion            = 1
)

type SyncSettings struct {
	Endpoint       string `json:"endpoint"`
	Username       string `json:"username"`
	AppPassword    string `json:"appPassword"`
	RemotePath     string `json:"remotePath"`
	SyncPassphrase string `json:"syncPassphrase"`
}

func DefaultSyncSettings() SyncSettings {
	return SyncSettings{RemotePath: DefaultRemotePath}
}

type UIPreferences struct {
	Theme  string `json:"theme"`
	Locale string `json:"locale"`
}

type BackendSnapshot struct {
	AppPreferences routingdomain.AppPreferences `json:"appPreferences"`
	ClaudeProfile  routingdomain.RouteProfile   `json:"claudeProfile"`
	CodexProfile   routingdomain.RouteProfile   `json:"codexProfile"`
	GeminiProfile  routingdomain.RouteProfile   `json:"geminiProfile"`
}

type ConfigSnapshot struct {
	Backend BackendSnapshot `json:"backend"`
	UI      UIPreferences   `json:"ui"`
}

type RemoteSnapshotMeta struct {
	Exists        bool   `json:"exists"`
	UpdatedAt     string `json:"updatedAt"`
	DeviceName    string `json:"deviceName"`
	AppVersion    string `json:"appVersion"`
	SchemaVersion int    `json:"schemaVersion"`
}
