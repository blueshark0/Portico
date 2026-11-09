package bootstrap

import "portico/internal/shared/storage"

func (i *Initializer) initConnections() error {
	appDBPath, err := storage.AppDataPath("app.db")
	if err != nil {
		return err
	}
	requestLogDBPath, err := storage.AppDataPath("request_log.db")
	if err != nil {
		return err
	}
	sessionDBPath, err := storage.AppDataPath("session.db")
	if err != nil {
		return err
	}

	return storage.OpenConnections([]storage.ConnectionConfig{
		{
			Name:        storage.CoreDBName,
			Driver:      "sqlite",
			DSN:         sqliteDSN(appDBPath),
			MaxOpenConn: 1,
			MaxIdleConn: 1,
		},
		{
			Name:        storage.RequestLogDBName,
			Driver:      "sqlite",
			DSN:         sqliteDSN(requestLogDBPath),
			MaxOpenConn: 1,
			MaxIdleConn: 1,
		},
		{
			Name:        storage.RequestLogReadDBName,
			Driver:      "sqlite",
			DSN:         sqliteDSN(requestLogDBPath),
			MaxOpenConn: 4,
			MaxIdleConn: 2,
		},
		{
			Name:        storage.SessionDBName,
			Driver:      "sqlite",
			DSN:         sqliteDSN(sessionDBPath),
			MaxOpenConn: 5,
			MaxIdleConn: 2,
		},
	})
}

func sqliteDSN(path string) string {
	return storage.SQLiteDSN(path)
}

func (i *Initializer) initReadConnections() error {
	path, err := storage.AppDataPath("request_log.db")
	if err != nil {
		return err
	}
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name: storage.RequestLogReadDBName, Driver: "sqlite",
		DSN: storage.SQLiteReadOnlyDSN(path), MaxOpenConn: 4, MaxIdleConn: 2,
	}); err != nil {
		return err
	}
	db, err := storage.DB(storage.RequestLogReadDBName)
	if err != nil {
		return err
	}
	return db.Ping()
}
