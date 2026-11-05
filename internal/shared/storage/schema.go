package storage

import (
	"database/sql"
	"fmt"
)

// SQLiteDSN 生成 modernc.org/sqlite 的连接串：_pragma 由驱动在每个新连接上
// 执行（mattn 风格的 _journal_mode/_busy_timeout 参数会被 modernc 静默丢弃）。
func SQLiteDSN(path string) string {
	return fmt.Sprintf("%s?_pragma=busy_timeout(15000)"+
		"&_pragma=journal_mode(WAL)"+
		"&_pragma=synchronous(NORMAL)"+
		"&_pragma=journal_size_limit(67108864)"+
		"&_pragma=wal_autocheckpoint(1000)", path)
}

// ColumnDef 描述补列迁移中的一个列。
type ColumnDef struct {
	Name       string
	Definition string
}

// EnsureColumns 为已存在的表补齐缺失列（轻量前向迁移）。
// table 与 Definition 必须来自编译期常量，不能拼接任何外部输入。
func EnsureColumns(db *sql.DB, table string, columns []ColumnDef) error {
	for _, column := range columns {
		var count int
		if err := db.QueryRow(
			fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", table),
			column.Name,
		).Scan(&count); err != nil {
			return fmt.Errorf("query %s.%s: %w", table, column.Name, err)
		}
		if count > 0 {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s %s",
			table, column.Name, column.Definition,
		)); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, column.Name, err)
		}
	}
	return nil
}
