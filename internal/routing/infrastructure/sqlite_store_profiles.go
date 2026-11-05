package infrastructure

import (
	"context"
	"database/sql"

	"portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func (s *SQLiteStore) GetProfile(ctx context.Context, platform kernel.Platform) (domain.RouteProfile, error) {
	db, err := queryDB(s.dbName)
	if err != nil {
		return domain.RouteProfile{}, err
	}
	profile := domain.RouteProfile{
		Platform:  platform,
		Providers: []domain.Provider{},
	}

	var defaultProviderID sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT default_provider_id FROM route_profiles WHERE platform = ?`, platform.String()).Scan(&defaultProviderID); err != nil {
		if err != sql.ErrNoRows {
			return domain.RouteProfile{}, err
		}
	} else if defaultProviderID.Valid {
		value := int(defaultProviderID.Int64)
		profile.DefaultProviderID = &value
	}

	rows, err := db.QueryContext(ctx, `SELECT id, name, api_url, api_key, official_site, tint, accent,
		position, supported_models_json, model_mapping_json, request_body_replacements_json, proxy_mode, proxy_url, insecure_skip_verify, archived, archived_at
		FROM providers
		WHERE platform = ?
		ORDER BY position ASC, id ASC`, platform.String())
	if err != nil {
		return domain.RouteProfile{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var provider domain.Provider
		var supportedModelsJSON string
		var modelMappingJSON string
		var requestBodyReplacementsJSON string
		var insecureSkipVerify int
		var archived int
		var archivedAt sql.NullTime
		if err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.APIURL,
			&provider.APIKey,
			&provider.OfficialSite,
			&provider.Tint,
			&provider.Accent,
			&provider.Position,
			&supportedModelsJSON,
			&modelMappingJSON,
			&requestBodyReplacementsJSON,
			&provider.ProxyMode,
			&provider.ProxyURL,
			&insecureSkipVerify,
			&archived,
			&archivedAt,
		); err != nil {
			return domain.RouteProfile{}, err
		}
		provider.InsecureSkipVerify = insecureSkipVerify == 1
		provider.Archived = archived == 1
		if archivedAt.Valid {
			value := archivedAt.Time.UTC()
			provider.ArchivedAt = &value
		}
		provider.SupportedModels = map[string]bool{}
		provider.ModelMapping = map[string]string{}
		if err := decodeJSONMap(supportedModelsJSON, &provider.SupportedModels); err != nil {
			return domain.RouteProfile{}, err
		}
		if err := decodeJSONMap(modelMappingJSON, &provider.ModelMapping); err != nil {
			return domain.RouteProfile{}, err
		}
		if err := decodeJSONMap(requestBodyReplacementsJSON, &provider.RequestBodyReplacements); err != nil {
			return domain.RouteProfile{}, err
		}
		profile.Providers = append(profile.Providers, provider)
	}
	if err := rows.Err(); err != nil {
		return domain.RouteProfile{}, err
	}
	return profile.Normalize(), nil
}

func (s *SQLiteStore) SaveProfile(ctx context.Context, profile domain.RouteProfile) (domain.RouteProfile, error) {
	db, err := queryDB(s.dbName)
	if err != nil {
		return domain.RouteProfile{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RouteProfile{}, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := saveProfileTx(ctx, tx, profile); err != nil {
		return domain.RouteProfile{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RouteProfile{}, err
	}
	tx = nil
	return s.GetProfile(ctx, profile.Platform)
}

// ReplaceAll 在单个事务里替换应用偏好与全部平台 profile：云端快照恢复
// 到一半失败时，不能留下 Claude 已是新配置、Codex 还是旧配置的混合状态。
func (s *SQLiteStore) ReplaceAll(ctx context.Context, preferences domain.AppPreferences, profiles []domain.RouteProfile) error {
	if err := domain.ValidateProxyURL(preferences.ProxyURL); err != nil {
		return err
	}
	db, err := queryDB(s.dbName)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := saveAppPreferencesIn(ctx, tx, preferences); err != nil {
		return err
	}
	for _, profile := range profiles {
		if err := saveProfileTx(ctx, tx, profile); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func saveProfileTx(ctx context.Context, tx *sql.Tx, profile domain.RouteProfile) error {
	var defaultProviderID any
	if profile.DefaultProviderID != nil {
		defaultProviderID = *profile.DefaultProviderID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO route_profiles(platform, default_provider_id, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(platform) DO UPDATE SET default_provider_id = excluded.default_provider_id, updated_at = CURRENT_TIMESTAMP`,
		profile.Platform.String(), defaultProviderID,
	); err != nil {
		return err
	}

	existingIDs, err := loadProviderIDs(ctx, tx, profile.Platform.String())
	if err != nil {
		return err
	}
	incomingIDs := make(map[int]struct{}, len(profile.Providers))
	for _, provider := range profile.Providers {
		incomingIDs[provider.ID] = struct{}{}
	}
	for _, providerID := range existingIDs {
		if _, ok := incomingIDs[providerID]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM providers WHERE platform = ? AND id = ?`, profile.Platform.String(), providerID); err != nil {
			return err
		}
	}
	for _, provider := range profile.Providers {
		supportedModelsJSON, err := encodeJSONMap(provider.SupportedModels)
		if err != nil {
			return err
		}
		modelMappingJSON, err := encodeJSONMap(provider.ModelMapping)
		if err != nil {
			return err
		}
		requestBodyReplacementsJSON, err := encodeJSONMap(provider.RequestBodyReplacements)
		if err != nil {
			return err
		}
		var archivedAt any
		if provider.Archived && provider.ArchivedAt != nil {
			archivedAt = provider.ArchivedAt.UTC()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO providers(
			platform, id, name, api_url, api_key, official_site, tint, accent, position,
			supported_models_json, model_mapping_json, request_body_replacements_json, proxy_mode, proxy_url, insecure_skip_verify, archived, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, id) DO UPDATE SET
			name = excluded.name,
			api_url = excluded.api_url,
			api_key = excluded.api_key,
			official_site = excluded.official_site,
			tint = excluded.tint,
			accent = excluded.accent,
			position = excluded.position,
			supported_models_json = excluded.supported_models_json,
			model_mapping_json = excluded.model_mapping_json,
			request_body_replacements_json = excluded.request_body_replacements_json,
			proxy_mode = excluded.proxy_mode,
			proxy_url = excluded.proxy_url,
			insecure_skip_verify = excluded.insecure_skip_verify,
			archived = excluded.archived,
			archived_at = CASE
				WHEN excluded.archived = 0 THEN NULL
				WHEN providers.archived = 0 THEN COALESCE(excluded.archived_at, CURRENT_TIMESTAMP)
				ELSE COALESCE(providers.archived_at, excluded.archived_at)
			END`,
			profile.Platform.String(),
			provider.ID,
			provider.Name,
			provider.APIURL,
			provider.APIKey,
			provider.OfficialSite,
			provider.Tint,
			provider.Accent,
			provider.Position,
			supportedModelsJSON,
			modelMappingJSON,
			requestBodyReplacementsJSON,
			provider.ProxyMode,
			provider.ProxyURL,
			boolToInt(provider.InsecureSkipVerify),
			boolToInt(provider.Archived),
			archivedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func loadProviderIDs(ctx context.Context, tx *sql.Tx, platform string) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM providers WHERE platform = ?`, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int, 0)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
