package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"

	_ "modernc.org/sqlite"
)

// ConnectionConfig describes a named database/sql connection pool.
type ConnectionConfig struct {
	Name        string
	Driver      string
	DSN         string
	MaxOpenConn int
	MaxIdleConn int
}

var connections = struct {
	sync.RWMutex
	dbs map[string]*sql.DB
	// order lists registered names, oldest first; see CloseConnections.
	order []string
}{dbs: make(map[string]*sql.DB)}

// OpenConnections opens and registers all connection pools. Pools opened by
// this call are closed again if a later configuration fails.
func OpenConnections(configs []ConnectionConfig) error {
	opened := make([]string, 0, len(configs))
	for _, config := range configs {
		if err := OpenConnection(config); err != nil {
			for _, name := range opened {
				_ = CloseConnection(name)
			}
			return err
		}
		opened = append(opened, config.Name)
	}
	return nil
}

// OpenConnection opens and registers a named database/sql connection pool.
func OpenConnection(config ConnectionConfig) error {
	if config.Name == "" {
		return errors.New("database connection name is empty")
	}
	if config.Driver == "" {
		return fmt.Errorf("database driver is empty for connection %q", config.Name)
	}

	db, err := sql.Open(config.Driver, config.DSN)
	if err != nil {
		return fmt.Errorf("open database connection %q: %w", config.Name, err)
	}
	maxOpen := config.MaxOpenConn
	if maxOpen == 0 {
		maxOpen = 100
	}
	maxIdle := config.MaxIdleConn
	if maxIdle == 0 {
		maxIdle = 20
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	connections.Lock()
	defer connections.Unlock()
	if _, exists := connections.dbs[config.Name]; exists {
		_ = db.Close()
		return fmt.Errorf("database connection %q is already registered", config.Name)
	}
	connections.dbs[config.Name] = db
	connections.order = append(connections.order, config.Name)
	return nil
}

// DB returns a previously registered connection pool.
func DB(name string) (*sql.DB, error) {
	connections.RLock()
	db, ok := connections.dbs[name]
	connections.RUnlock()
	if !ok {
		return nil, fmt.Errorf("database connection %q is not registered", name)
	}
	return db, nil
}

// CloseConnection unregisters and closes one connection pool. Unknown names
// are ignored so cleanup remains idempotent.
func CloseConnection(name string) error {
	connections.Lock()
	db, ok := connections.dbs[name]
	if ok {
		delete(connections.dbs, name)
		connections.order = slices.DeleteFunc(connections.order, func(n string) bool { return n == name })
	}
	connections.Unlock()
	if !ok {
		return nil
	}
	return db.Close()
}

// CloseConnections closes every registered connection pool, newest first. A
// read-only pool opens after the writer on the same file and must close before
// it: SQLite only checkpoints and removes -wal/-shm when the last connection to
// close can write.
func CloseConnections() error {
	connections.Lock()
	dbs := connections.dbs
	order := connections.order
	connections.dbs = make(map[string]*sql.DB)
	connections.order = nil
	connections.Unlock()

	errs := make([]error, 0, len(dbs))
	for _, name := range slices.Backward(order) {
		if err := dbs[name].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close database connection %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
