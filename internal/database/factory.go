package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var driverRegistry = make(map[Dialect]Driver)

// RegisterDriver registers a driver instance for a dialect
func RegisterDriver(dialect Dialect, driver Driver) {
	driverRegistry[dialect] = driver
}

// GetDriver returns the driver for the specified dialect
func GetDriver(dialect Dialect) (Driver, error) {
	d, ok := driverRegistry[dialect]
	if !ok {
		return nil, fmt.Errorf("unsupported database dialect: %s (supported: mysql, mariadb, postgres, sqlite)", dialect)
	}
	return d, nil
}

// OpenConnection opens a database connection based on config
func OpenConnection(ctx context.Context, cfg *ConnectionConfig) (*sql.DB, Driver, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("connection configuration cannot be nil")
	}

	driver, err := GetDriver(cfg.Dialect)
	if err != nil {
		return nil, nil, err
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	db, err := driver.Connect(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	return db, driver, nil
}

// ParseURL parses a database connection URI into a ConnectionConfig
func ParseURL(rawURL string) (*ConnectionConfig, error) {
	// Check for sqlite file path directly
	if strings.HasPrefix(rawURL, "sqlite://") {
		path := strings.TrimPrefix(rawURL, "sqlite://")
		return &ConnectionConfig{
			Dialect:  DialectSQLite,
			FilePath: path,
			Database: path,
		}, nil
	}
	if strings.HasSuffix(rawURL, ".db") || strings.HasSuffix(rawURL, ".sqlite") || strings.HasSuffix(rawURL, ".sqlite3") {
		return &ConnectionConfig{
			Dialect:  DialectSQLite,
			FilePath: rawURL,
			Database: rawURL,
		}, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection URL: %w", err)
	}

	cfg := &ConnectionConfig{
		Host:     u.Hostname(),
		Database: strings.TrimPrefix(u.Path, "/"),
	}

	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err == nil {
			cfg.Port = port
		}
	}

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	q := u.Query()
	if ssl := q.Get("sslmode"); ssl != "" {
		cfg.SSLMode = ssl
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "mysql":
		cfg.Dialect = DialectMySQL
		if cfg.Port == 0 {
			cfg.Port = 3306
		}
	case "mariadb":
		cfg.Dialect = DialectMariaDB
		if cfg.Port == 0 {
			cfg.Port = 3306
		}
	case "postgres", "postgresql":
		cfg.Dialect = DialectPostgreSQL
		if cfg.Port == 0 {
			cfg.Port = 5432
		}
	case "sqlite":
		cfg.Dialect = DialectSQLite
		cfg.FilePath = u.Path
	default:
		return nil, fmt.Errorf("unsupported database scheme in URL: %s", scheme)
	}

	return cfg, nil
}
