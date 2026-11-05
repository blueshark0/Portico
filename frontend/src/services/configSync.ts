import {
  ApplySnapshot,
  CreateLocalRollback,
  GetRemoteSnapshotMeta,
  GetSyncSettings,
  PullSnapshot,
  PushSnapshot,
  SaveSyncSettings,
  TestConnection,
} from '../../bindings/portico/internal/interfaces/wails/configsyncfacade'
import type {
  BackendSnapshot,
  ConfigSnapshot,
  RemoteSnapshotMeta,
  SyncSettings,
  UIPreferences,
} from '../../bindings/portico/internal/configsync/domain/models'

export type { BackendSnapshot, ConfigSnapshot, RemoteSnapshotMeta, SyncSettings, UIPreferences }

export const fetchSyncSettings = async (): Promise<SyncSettings> => GetSyncSettings()

export const saveSyncSettings = async (settings: SyncSettings): Promise<SyncSettings> => {
  return SaveSyncSettings(settings)
}

export const testConnection = async (): Promise<void> => {
  await TestConnection()
}

export const fetchRemoteSnapshotMeta = async (): Promise<RemoteSnapshotMeta> => GetRemoteSnapshotMeta()

export const pushSnapshot = async (ui: UIPreferences): Promise<RemoteSnapshotMeta> => PushSnapshot(ui)

export const pullSnapshot = async (): Promise<ConfigSnapshot> => PullSnapshot()

export const createLocalRollback = async (ui: UIPreferences): Promise<string> => {
  return CreateLocalRollback(ui)
}

export const applySnapshot = async (snapshot: BackendSnapshot): Promise<void> => {
  await ApplySnapshot(snapshot)
}
