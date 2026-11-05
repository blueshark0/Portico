package infrastructure

import (
	"context"
	"database/sql"
)

func (s *SQLiteStore) HasLegacyImport(ctx context.Context) (bool, error) {
	return s.metaEquals(ctx, legacyImportMetaKey, "1")
}

func (s *SQLiteStore) MarkLegacyImported(ctx context.Context) error {
	db, err := queryDB(s.dbName)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO schema_meta(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, legacyImportMetaKey, "1")
	return err
}

func (s *SQLiteStore) metaEquals(ctx context.Context, key string, want string) (bool, error) {
	db, err := queryDB(s.dbName)
	if err != nil {
		return false, err
	}
	var value string
	if err := db.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key = ?`, key).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return value == want, nil
}
