package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"portico/internal/mcpgateway/domain"
	"portico/internal/shared/storage"
)

// SQLiteStore mcp_servers 表的 SQLite 实现，落在 app.db（storage.CoreDBName）。
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
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS mcp_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL DEFAULT 'stdio',
		command TEXT NOT NULL DEFAULT '',
		args_json TEXT NOT NULL DEFAULT '[]',
		env_json TEXT NOT NULL DEFAULT '{}',
		url TEXT NOT NULL DEFAULT '',
		headers_json TEXT NOT NULL DEFAULT '{}',
		enabled INTEGER NOT NULL DEFAULT 1,
		position INTEGER NOT NULL DEFAULT 0,
		timeout_sec INTEGER NOT NULL DEFAULT 30,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (s *SQLiteStore) db() (*sql.DB, error) {
	return storage.DB(s.dbName)
}

// List 按 position 升序返回全部 MCP 服务配置。
func (s *SQLiteStore) List(ctx context.Context) ([]domain.McpServer, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name, type, command, args_json, env_json,
		url, headers_json, enabled, position, timeout_sec, created_at, updated_at
		FROM mcp_servers ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []domain.McpServer
	for rows.Next() {
		srv, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanServer(row rowScanner) (domain.McpServer, error) {
	var srv domain.McpServer
	var argsJSON, envJSON, headersJSON string
	var enabled int
	err := row.Scan(&srv.ID, &srv.Name, &srv.Type, &srv.Command, &argsJSON, &envJSON,
		&srv.URL, &headersJSON, &enabled, &srv.Position, &srv.TimeoutSec, &srv.CreatedAt, &srv.UpdatedAt)
	if err != nil {
		return srv, err
	}
	srv.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(argsJSON), &srv.Args); err != nil {
		return srv, fmt.Errorf("decode args_json for %q: %w", srv.Name, err)
	}
	if err := json.Unmarshal([]byte(envJSON), &srv.Env); err != nil {
		return srv, fmt.Errorf("decode env_json for %q: %w", srv.Name, err)
	}
	if err := json.Unmarshal([]byte(headersJSON), &srv.Headers); err != nil {
		return srv, fmt.Errorf("decode headers_json for %q: %w", srv.Name, err)
	}
	return srv, nil
}

func (s *SQLiteStore) Create(ctx context.Context, srv *domain.McpServer) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	argsJSON, envJSON, headersJSON, err := encodeFields(srv)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `INSERT INTO mcp_servers
		(name, type, command, args_json, env_json, url, headers_json, enabled, position, timeout_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, string(srv.Type), srv.Command, argsJSON, envJSON, srv.URL, headersJSON,
		boolToInt(srv.Enabled), srv.Position, srv.TimeoutSec, now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	srv.ID = id
	srv.CreatedAt = now
	srv.UpdatedAt = now
	return nil
}

func (s *SQLiteStore) Update(ctx context.Context, srv domain.McpServer) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	argsJSON, envJSON, headersJSON, err := encodeFields(&srv)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE mcp_servers SET
		name = ?, type = ?, command = ?, args_json = ?, env_json = ?, url = ?, headers_json = ?,
		enabled = ?, position = ?, timeout_sec = ?, updated_at = ?
		WHERE id = ?`,
		srv.Name, string(srv.Type), srv.Command, argsJSON, envJSON, srv.URL, headersJSON,
		boolToInt(srv.Enabled), srv.Position, srv.TimeoutSec, time.Now().UTC(), srv.ID)
	return err
}

func (s *SQLiteStore) Delete(ctx context.Context, id int64) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE mcp_servers SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), time.Now().UTC(), id)
	return err
}

func encodeFields(srv *domain.McpServer) (argsJSON, envJSON, headersJSON string, err error) {
	if argsJSON, err = encodeJSON(srv.Args, "[]"); err != nil {
		return "", "", "", err
	}
	if envJSON, err = encodeJSON(srv.Env, "{}"); err != nil {
		return "", "", "", err
	}
	if headersJSON, err = encodeJSON(srv.Headers, "{}"); err != nil {
		return "", "", "", err
	}
	return argsJSON, envJSON, headersJSON, nil
}

func encodeJSON(value any, emptyDefault string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(data) == "null" {
		return emptyDefault, nil
	}
	return string(data), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
