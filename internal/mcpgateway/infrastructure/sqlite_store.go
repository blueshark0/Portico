package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	if err != nil {
		return err
	}
	if err := storage.EnsureColumns(db, "mcp_servers", []storage.ColumnDef{
		{Name: "auth_type", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "oauth_client_name", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "oauth_client_id", Definition: "TEXT NOT NULL DEFAULT ''"},
		{Name: "oauth_client_secret", Definition: "TEXT NOT NULL DEFAULT ''"},
	}); err != nil {
		return err
	}
	// 令牌与 headers_json 中的静态凭证同级：明文落在 app.db，不参与 configsync。
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS mcp_oauth_credentials (
		server_id INTEGER PRIMARY KEY,
		client_id TEXT NOT NULL DEFAULT '',
		client_secret TEXT NOT NULL DEFAULT '',
		auth_style INTEGER NOT NULL DEFAULT 0,
		auth_url TEXT NOT NULL DEFAULT '',
		token_url TEXT NOT NULL DEFAULT '',
		redirect_url TEXT NOT NULL DEFAULT '',
		scopes_json TEXT NOT NULL DEFAULT '[]',
		access_token TEXT NOT NULL DEFAULT '',
		refresh_token TEXT NOT NULL DEFAULT '',
		token_type TEXT NOT NULL DEFAULT '',
		expiry_unix INTEGER NOT NULL DEFAULT 0,
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
		url, headers_json, enabled, position, timeout_sec, created_at, updated_at,
		auth_type, oauth_client_name, oauth_client_id, oauth_client_secret
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
		&srv.URL, &headersJSON, &enabled, &srv.Position, &srv.TimeoutSec, &srv.CreatedAt, &srv.UpdatedAt,
		&srv.AuthType, &srv.OAuthClientName, &srv.OAuthClientID, &srv.OAuthClientSecret)
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
		(name, type, command, args_json, env_json, url, headers_json, enabled, position, timeout_sec, created_at, updated_at,
		auth_type, oauth_client_name, oauth_client_id, oauth_client_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, string(srv.Type), srv.Command, argsJSON, envJSON, srv.URL, headersJSON,
		boolToInt(srv.Enabled), srv.Position, srv.TimeoutSec, now, now,
		string(srv.AuthType), srv.OAuthClientName, srv.OAuthClientID, srv.OAuthClientSecret)
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
		enabled = ?, position = ?, timeout_sec = ?, updated_at = ?,
		auth_type = ?, oauth_client_name = ?, oauth_client_id = ?, oauth_client_secret = ?
		WHERE id = ?`,
		srv.Name, string(srv.Type), srv.Command, argsJSON, envJSON, srv.URL, headersJSON,
		boolToInt(srv.Enabled), srv.Position, srv.TimeoutSec, time.Now().UTC(),
		string(srv.AuthType), srv.OAuthClientName, srv.OAuthClientID, srv.OAuthClientSecret, srv.ID)
	return err
}

func (s *SQLiteStore) Delete(ctx context.Context, id int64) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	// 服务与其 OAuth 凭证同生共灭：留下孤儿令牌既无用又是多余的敏感数据。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_oauth_credentials WHERE server_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// GetOAuthCredential 读取某个服务的 OAuth 凭证；不存在时返回 (nil, nil)。
func (s *SQLiteStore) GetOAuthCredential(ctx context.Context, serverID int64) (*domain.OAuthCredential, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	cred := domain.OAuthCredential{ServerID: serverID}
	var scopesJSON string
	var expiryUnix int64
	err = db.QueryRowContext(ctx, `SELECT client_id, client_secret, auth_style, auth_url, token_url,
		redirect_url, scopes_json, access_token, refresh_token, token_type, expiry_unix
		FROM mcp_oauth_credentials WHERE server_id = ?`, serverID).Scan(
		&cred.ClientID, &cred.ClientSecret, &cred.AuthStyle, &cred.AuthURL, &cred.TokenURL,
		&cred.RedirectURL, &scopesJSON, &cred.AccessToken, &cred.RefreshToken, &cred.TokenType, &expiryUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopesJSON), &cred.Scopes); err != nil {
		return nil, fmt.Errorf("decode scopes_json for server #%d: %w", serverID, err)
	}
	if expiryUnix > 0 {
		cred.Expiry = time.Unix(expiryUnix, 0).UTC()
	}
	return &cred, nil
}

// SaveOAuthCredential 写入（覆盖）某个服务的 OAuth 凭证。
func (s *SQLiteStore) SaveOAuthCredential(ctx context.Context, cred domain.OAuthCredential) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	scopesJSON, err := encodeJSON(cred.Scopes, "[]")
	if err != nil {
		return err
	}
	var expiryUnix int64
	if !cred.Expiry.IsZero() {
		expiryUnix = cred.Expiry.Unix()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO mcp_oauth_credentials
		(server_id, client_id, client_secret, auth_style, auth_url, token_url, redirect_url,
		scopes_json, access_token, refresh_token, token_type, expiry_unix, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id) DO UPDATE SET
		client_id = excluded.client_id, client_secret = excluded.client_secret,
		auth_style = excluded.auth_style, auth_url = excluded.auth_url, token_url = excluded.token_url,
		redirect_url = excluded.redirect_url, scopes_json = excluded.scopes_json,
		access_token = excluded.access_token, refresh_token = excluded.refresh_token,
		token_type = excluded.token_type, expiry_unix = excluded.expiry_unix, updated_at = excluded.updated_at`,
		cred.ServerID, cred.ClientID, cred.ClientSecret, cred.AuthStyle, cred.AuthURL, cred.TokenURL,
		cred.RedirectURL, scopesJSON, cred.AccessToken, cred.RefreshToken, cred.TokenType, expiryUnix,
		time.Now().UTC())
	return err
}

// DeleteOAuthCredential 删除某个服务的 OAuth 凭证（幂等）。
func (s *SQLiteStore) DeleteOAuthCredential(ctx context.Context, serverID int64) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM mcp_oauth_credentials WHERE server_id = ?`, serverID)
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
