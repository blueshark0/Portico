package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"portico/internal/routing/domain"
)

// migrateModelMappingSchemes performs the one-time legacy backfill. The
// completion marker is important: an intentionally empty scheme list must not
// be repopulated from the compatibility mirror on the next application start.
func migrateModelMappingSchemes(db *sql.DB) error {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var marker string
	err = tx.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key = ?`, mappingSchemesMigratedMetaKey).Scan(&marker)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && marker == "1" {
		return nil
	}

	rows, err := tx.QueryContext(ctx, `SELECT platform, id, model_mapping_json, model_mapping_schemes_json FROM providers`)
	if err != nil {
		return err
	}
	type legacyRow struct {
		platform    string
		providerID  int
		mappingJSON string
		schemesJSON string
	}
	legacyRows := make([]legacyRow, 0)
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.platform, &row.providerID, &row.mappingJSON, &row.schemesJSON); err != nil {
			_ = rows.Close()
			return err
		}
		legacyRows = append(legacyRows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, row := range legacyRows {
		schemesJSON := strings.TrimSpace(row.schemesJSON)
		if schemesJSON != "" && schemesJSON != "[]" && schemesJSON != "null" {
			continue
		}
		mapping := map[string]string{}
		if strings.TrimSpace(row.mappingJSON) != "" {
			if err := json.Unmarshal([]byte(row.mappingJSON), &mapping); err != nil {
				return fmt.Errorf("decode legacy model mapping for %s/%d: %w", row.platform, row.providerID, err)
			}
		}
		if len(mapping) == 0 {
			continue
		}
		encoded, err := encodeJSONMap([]domain.ModelMappingScheme{{
			Name:    domain.DefaultModelMappingSchemeName,
			Mapping: mapping,
		}})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE providers
			SET model_mapping_schemes_json = ?, active_model_mapping_scheme = ?
			WHERE platform = ? AND id = ?`, encoded, domain.DefaultModelMappingSchemeName, row.platform, row.providerID); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_meta(key, value) VALUES (?, '1')
		ON CONFLICT(key) DO UPDATE SET value = '1'`, mappingSchemesMigratedMetaKey); err != nil {
		return err
	}
	return tx.Commit()
}
