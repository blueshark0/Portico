package infrastructure

import (
	"context"
	"database/sql"

	configsyncdomain "portico/internal/configsync/domain"
	"portico/internal/shared/storage"
)

type SQLiteStore struct {
	dbName string
}

func NewSQLiteStore() *SQLiteStore {
	return &SQLiteStore{dbName: storage.CoreDBName}
}

func NewSQLiteStoreWithDBName(dbName string) *SQLiteStore {
	if dbName == "" {
		dbName = storage.CoreDBName
	}
	return &SQLiteStore{dbName: dbName}
}

func (s *SQLiteStore) EnsureSchema() error {
	db, err := storage.DB(s.dbName)
	if err != nil {
		return err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS sync_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		endpoint TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT '',
		app_password TEXT NOT NULL DEFAULT '',
		remote_path TEXT NOT NULL DEFAULT '/Portico',
		sync_passphrase TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if err := ensureSyncSettingsColumns(db); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO sync_settings (id, endpoint, username, app_password, remote_path, sync_passphrase)
		VALUES (1, '', '', '', '/Portico', '')
		ON CONFLICT(id) DO NOTHING`)
	return err
}

func ensureSyncSettingsColumns(db *sql.DB) error {
	return storage.EnsureColumns(db, "sync_settings", []storage.ColumnDef{
		{Name: "endpoint", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "username", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "app_password", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "remote_path", Definition: "TEXT NOT NULL DEFAULT '/Portico'"},
		{Name: "sync_passphrase", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "updated_at", Definition: "DATETIME"},
	})
}

func (s *SQLiteStore) GetSettings(ctx context.Context) (configsyncdomain.SyncSettings, error) {
	db, err := storage.DB(s.dbName)
	if err != nil {
		return configsyncdomain.SyncSettings{}, err
	}
	settings := configsyncdomain.DefaultSyncSettings()
	if err := db.QueryRowContext(ctx, `SELECT endpoint, username, app_password, remote_path, sync_passphrase
		FROM sync_settings WHERE id = 1`).Scan(
		&settings.Endpoint,
		&settings.Username,
		&settings.AppPassword,
		&settings.RemotePath,
		&settings.SyncPassphrase,
	); err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return configsyncdomain.SyncSettings{}, err
	}
	return settings, nil
}

func (s *SQLiteStore) SaveSettings(ctx context.Context, settings configsyncdomain.SyncSettings) (configsyncdomain.SyncSettings, error) {
	db, err := storage.DB(s.dbName)
	if err != nil {
		return configsyncdomain.SyncSettings{}, err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_settings(id, endpoint, username, app_password, remote_path, sync_passphrase, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			endpoint = excluded.endpoint,
			username = excluded.username,
			app_password = excluded.app_password,
			remote_path = excluded.remote_path,
			sync_passphrase = excluded.sync_passphrase,
			updated_at = CURRENT_TIMESTAMP`,
		settings.Endpoint,
		settings.Username,
		settings.AppPassword,
		settings.RemotePath,
		settings.SyncPassphrase,
	); err != nil {
		return configsyncdomain.SyncSettings{}, err
	}
	return s.GetSettings(ctx)
}
