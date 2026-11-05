package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	configsyncinfra "portico/internal/configsync/infrastructure"
	mcpgatewayinfra "portico/internal/mcpgateway/infrastructure"
	observabilityinfra "portico/internal/observability/infrastructure"
	routinginfra "portico/internal/routing/infrastructure"
	sessionsinfra "portico/internal/sessions/infrastructure"
	"portico/internal/shared/storage"
)

type Initializer struct {
	dbDir       string
	initialized bool
	mu          sync.Mutex
}

func NewInitializer() *Initializer {
	dbDir, _ := storage.AppDataDir()
	return &Initializer{dbDir: dbDir}
}

func (i *Initializer) Initialize() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.initialized {
		return nil
	}
	if err := os.MkdirAll(i.dbDir, 0o755); err != nil {
		return fmt.Errorf("创建数据库目录失败: %w", err)
	}
	if err := i.initConnections(); err != nil {
		return err
	}
	if err := i.ensureSchemas(); err != nil {
		_ = storage.CloseConnections()
		return err
	}
	if err := i.runMigrations(); err != nil {
		log.Printf("数据迁移警告: %v\n", err)
	}
	i.initialized = true
	i.runStartupMaintenanceAsync()
	return nil
}

func (i *Initializer) ensureSchemas() error {
	if err := routinginfra.NewSQLiteStore().EnsureSchema(); err != nil {
		return fmt.Errorf("初始化 app 配置表失败: %w", err)
	}
	if err := configsyncinfra.NewSQLiteStore().EnsureSchema(); err != nil {
		return fmt.Errorf("初始化同步配置表失败: %w", err)
	}
	if err := ensureRequestLogSchema(storage.RequestLogDBName); err != nil {
		return fmt.Errorf("初始化 request_log 表失败: %w", err)
	}
	if err := sessionsinfra.NewSQLiteRepository(storage.SessionDBName).EnsureSchema(); err != nil {
		return fmt.Errorf("初始化 session_provider_binding 表失败: %w", err)
	}
	if err := mcpgatewayinfra.NewSQLiteStore().EnsureSchema(); err != nil {
		return fmt.Errorf("初始化 mcp_servers 表失败: %w", err)
	}
	return nil
}

func (i *Initializer) runMigrations() error {
	requestLogDBPath, err := storage.AppDataPath("request_log.db")
	if err != nil {
		return err
	}
	sessionDBPath, err := storage.AppDataPath("session.db")
	if err != nil {
		return err
	}
	store := routinginfra.NewSQLiteStore()
	errs := []error{
		migrateLegacyTables(storage.CoreDBName, storage.RequestLogDBName, requestLogDBPath, "request_log"),
		migrateLegacyTables(storage.CoreDBName, storage.SessionDBName, sessionDBPath, "session_provider_binding"),
	}
	if err := routinginfra.NewLegacyImporter(store).EnsureImported(context.Background()); err != nil {
		errs = append(errs, fmt.Errorf("legacy config import: %w", err))
	}
	return errors.Join(errs...)
}

func (i *Initializer) runStartupMaintenanceAsync() {
	go func() {
		if err := observabilityinfra.CleanupOldRequestLogs(observabilityinfra.RequestLogRetentionDays); err != nil {
			log.Printf("启动维护警告: %v\n", err)
		}
	}()
}
