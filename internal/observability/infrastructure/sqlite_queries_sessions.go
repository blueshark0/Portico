package infrastructure

import (
	"database/sql"
	"strings"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/storage"
)

// SessionUsageForSessions aggregates only the active bindings requested by the
// session page. Empty input never opens the database; IDs are batched to stay
// below SQLite's parameter limit without changing full-retention usage totals.
func (q *SQLiteQueries) SessionUsageForSessions(platform string, sessionIDs []string) (map[string]observabilitydomain.SessionUsage, error) {
	result := make(map[string]observabilitydomain.SessionUsage)
	if q == nil || strings.TrimSpace(platform) == "" {
		return result, nil
	}
	seen := make(map[string]bool, len(sessionIDs))
	ids := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return result, nil
	}

	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return nil, err
	}
	const batchSize = 500
	for start := 0; start < len(ids); start += batchSize {
		if err := querySessionUsage(db, platform, ids[start:min(start+batchSize, len(ids))], result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func querySessionUsage(db *sql.DB, platform string, ids []string, result map[string]observabilitydomain.SessionUsage) error {
	args := make([]any, 1, len(ids)+1)
	args[0] = platform
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := db.Query(`
		SELECT session_id,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(total_cost), 0) AS total_cost,
			GROUP_CONCAT(DISTINCT model) AS models
		FROM request_log
		WHERE platform = ? AND session_id IN (`+placeholders+`)
		GROUP BY session_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var usage observabilitydomain.SessionUsage
		var models string
		if err := rows.Scan(
			&usage.SessionID,
			&usage.Requests,
			&usage.InputTokens,
			&usage.OutputTokens,
			&usage.TotalCost,
			&models,
		); err != nil {
			return err
		}
		usage.Models = splitDistinctModels(models)
		result[usage.SessionID] = usage
	}
	return rows.Err()
}

func splitDistinctModels(models string) []string {
	parts := strings.Split(models, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
