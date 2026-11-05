package infrastructure

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	hotkeydomain "portico/internal/hotkeys/domain"
	"portico/internal/shared/storage"

	_ "modernc.org/sqlite"
)

const hotkeysDBFile = "hotkeys.db"

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore() (*SQLiteStore, error) {
	dbPath, err := storage.AppDataPath(hotkeysDBFile)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", storage.SQLiteDSN(dbPath))
	if err != nil {
		return nil, err
	}
	// 与其他库保持一致的单写者模型：无连接上限时并发写同一 SQLite 文件
	// 会直接得到 database is locked。
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{db: db}
	if err := store.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrateLegacyIfNeeded(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) List() ([]hotkeydomain.Hotkey, error) {
	rows, err := s.db.Query(`SELECT id, keycode, modifiers FROM hotkeys ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hotkeys := make([]hotkeydomain.Hotkey, 0)
	for rows.Next() {
		var hk hotkeydomain.Hotkey
		if err := rows.Scan(&hk.ID, &hk.KeyCode, &hk.Modifiers); err != nil {
			return nil, err
		}
		hotkeys = append(hotkeys, hk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hotkeys, nil
}

func (s *SQLiteStore) Update(id int, key int, modifier int) error {
	_, err := s.db.Exec(`
		UPDATE hotkeys
		SET keycode = ?, modifiers = ?
		WHERE id = ?
	`, key, modifier, id)
	return err
}

func (s *SQLiteStore) ensureSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS hotkeys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keycode INTEGER NOT NULL,
			modifiers INTEGER NOT NULL
		);
	`)
	return err
}

func (s *SQLiteStore) migrateLegacyIfNeeded() error {
	count, err := sqliteRowCount(s.db, "hotkeys")
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	legacyPath, err := storage.LegacyHotkeyDBPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		return err
	}
	defer legacyDB.Close()

	exists, err := sqliteTableExists(legacyDB, "hotkeys")
	if err != nil || !exists {
		return err
	}

	rows, err := legacyDB.Query(`SELECT id, keycode, modifiers FROM hotkeys ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	for rows.Next() {
		var (
			id        int
			keycode   int
			modifiers int
		)
		if err := rows.Scan(&id, &keycode, &modifiers); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO hotkeys (id, keycode, modifiers) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET keycode = excluded.keycode, modifiers = excluded.modifiers`,
			id,
			keycode,
			modifiers,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE sqlite_sequence SET seq = (SELECT COALESCE(MAX(id), 0) FROM hotkeys) WHERE name = 'hotkeys'`); err != nil {
		return fmt.Errorf("update hotkey sequence: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func sqliteTableExists(db *sql.DB, table string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func sqliteRowCount(db *sql.DB, table string) (int64, error) {
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
