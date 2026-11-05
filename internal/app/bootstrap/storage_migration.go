package bootstrap

import (
	"database/sql"
	"fmt"
	"strings"

	observabilityinfra "portico/internal/observability/infrastructure"
	"portico/internal/shared/storage"
)

func migrateLegacyTables(srcDBName, destDBName, destPath, table string) error {
	if destPath == "" {
		return fmt.Errorf("目标数据库路径为空: %s", table)
	}
	srcDB, err := storage.DB(srcDBName)
	if err != nil {
		return err
	}
	destDB, err := storage.DB(destDBName)
	if err != nil {
		return err
	}
	exists, err := sqliteTableExists(srcDB, table)
	if err != nil || !exists {
		return err
	}
	destRows, err := sqliteRowCount(destDB, table)
	if err != nil {
		return err
	}
	if destRows > 0 {
		return nil
	}
	srcRows, err := sqliteRowCount(srcDB, table)
	if err != nil {
		return err
	}
	if srcRows == 0 {
		return nil
	}

	alias := fmt.Sprintf("%s_migrate", table)
	if err := attachDatabase(srcDB, alias, destPath); err != nil {
		return err
	}
	defer detachDatabase(srcDB, alias)

	if _, err := srcDB.Exec(fmt.Sprintf("INSERT INTO %s.%s SELECT * FROM main.%s", alias, table, table)); err != nil {
		return err
	}
	return nil
}

func sqliteTableExists(db *sql.DB, table string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func sqliteRowCount(db *sql.DB, table string) (int64, error) {
	var count int64
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		if isNoSuchTableErr(err) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

func attachDatabase(db *sql.DB, alias, path string) error {
	if alias == "" || path == "" {
		return fmt.Errorf("attach 参数无效")
	}
	escaped := strings.ReplaceAll(path, "'", "''")
	_, err := db.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS %s", escaped, alias))
	return err
}

func detachDatabase(db *sql.DB, alias string) {
	if alias == "" {
		return
	}
	_, _ = db.Exec(fmt.Sprintf("DETACH DATABASE %s", alias))
}

func isNoSuchTableErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}

func ensureRequestLogSchema(dbName string) error {
	db, err := storage.DB(dbName)
	if err != nil {
		return err
	}
	return observabilityinfra.EnsureRequestLogTableWithDB(db)
}
