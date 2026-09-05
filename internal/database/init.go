package database

import (
	"sync"
)

var initOnce sync.Once

// InitRegistry ensures all built-in drivers are registered
func InitRegistry(mysqlDriver, mariadbDriver, postgresDriver, sqliteDriver Driver) {
	initOnce.Do(func() {
		if mysqlDriver != nil {
			RegisterDriver(DialectMySQL, mysqlDriver)
		}
		if mariadbDriver != nil {
			RegisterDriver(DialectMariaDB, mariadbDriver)
		}
		if postgresDriver != nil {
			RegisterDriver(DialectPostgreSQL, postgresDriver)
		}
		if sqliteDriver != nil {
			RegisterDriver(DialectSQLite, sqliteDriver)
		}
	})
}
