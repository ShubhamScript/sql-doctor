package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sql-doctor/sql-doctor/internal/database"
	_ "modernc.org/sqlite"
)

// Storage represents the local SQLite metadata repository
type Storage struct {
	db *sql.DB
}

// ConnectionRecord represents a persisted connection profile
type ConnectionRecord struct {
	Name      string           `json:"name"`
	Dialect   database.Dialect `json:"dialect"`
	Host      string           `json:"host"`
	Port      int              `json:"port"`
	User      string           `json:"user"`
	Password  string           `json:"password,omitempty"`
	Database  string           `json:"database"`
	FilePath  string           `json:"file_path,omitempty"`
	SSLMode   string           `json:"ssl_mode,omitempty"`
	IsActive  bool             `json:"is_active"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// QueryHistoryRecord stores historical query execution metrics
type QueryHistoryRecord struct {
	ID               int64     `json:"id"`
	ConnectionName   string    `json:"connection_name"`
	QueryText        string    `json:"query_text"`
	Fingerprint      string    `json:"fingerprint"`
	DurationMs       float64   `json:"duration_ms"`
	RowsExamined     int64     `json:"rows_examined"`
	RowsReturned     int64     `json:"rows_returned"`
	PerformanceScore int       `json:"performance_score"`
	CreatedAt        time.Time `json:"created_at"`
}

// SnapshotRecord stores a schema snapshot
type SnapshotRecord struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	ConnectionName string    `json:"connection_name"`
	DatabaseName   string    `json:"database_name"`
	Data           string    `json:"data"` // JSON encoded schema
	CreatedAt      time.Time `json:"created_at"`
}

// OpenStorage initializes the local SQLite database
func OpenStorage(customPath string) (*Storage, error) {
	dbPath := customPath
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		appDir := filepath.Join(homeDir, ".sql-doctor")
		if err := os.MkdirAll(appDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}
		dbPath = filepath.Join(appDir, "sql-doctor.db")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local sqlite storage: %w", err)
	}

	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate local storage: %w", err)
	}

	return s, nil
}

func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Storage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS connections (
		name TEXT PRIMARY KEY,
		dialect TEXT NOT NULL,
		host TEXT,
		port INTEGER,
		user TEXT,
		password TEXT,
		database_name TEXT,
		file_path TEXT,
		ssl_mode TEXT,
		is_active INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS query_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		connection_name TEXT,
		query_text TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		duration_ms REAL,
		rows_examined INTEGER,
		rows_returned INTEGER,
		performance_score INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		connection_name TEXT,
		database_name TEXT,
		data TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// SaveConnection creates or updates a connection profile
func (s *Storage) SaveConnection(ctx context.Context, conn *ConnectionRecord) error {
	query := `
		INSERT INTO connections (name, dialect, host, port, user, password, database_name, file_path, ssl_mode, is_active, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			dialect=excluded.dialect,
			host=excluded.host,
			port=excluded.port,
			user=excluded.user,
			password=excluded.password,
			database_name=excluded.database_name,
			file_path=excluded.file_path,
			ssl_mode=excluded.ssl_mode,
			is_active=excluded.is_active,
			updated_at=CURRENT_TIMESTAMP;
	`
	activeInt := 0
	if conn.IsActive {
		activeInt = 1
	}

	_, err := s.db.ExecContext(ctx, query,
		conn.Name, string(conn.Dialect), conn.Host, conn.Port, conn.User, conn.Password,
		conn.Database, conn.FilePath, conn.SSLMode, activeInt,
	)
	return err
}

// GetConnection returns a connection profile by name
func (s *Storage) GetConnection(ctx context.Context, name string) (*ConnectionRecord, error) {
	query := `
		SELECT name, dialect, host, port, user, password, database_name, file_path, ssl_mode, is_active, created_at, updated_at
		FROM connections WHERE name = ?;
	`
	row := s.db.QueryRowContext(ctx, query, name)

	var c ConnectionRecord
	var dialectStr string
	var activeInt int
	var createdStr, updatedStr string

	err := row.Scan(&c.Name, &dialectStr, &c.Host, &c.Port, &c.User, &c.Password, &c.Database, &c.FilePath, &c.SSLMode, &activeInt, &createdStr, &updatedStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connection profile '%s' not found", name)
		}
		return nil, err
	}
	c.Dialect = database.Dialect(dialectStr)
	c.IsActive = activeInt == 1
	return &c, nil
}

// GetActiveConnection returns the currently active connection profile
func (s *Storage) GetActiveConnection(ctx context.Context) (*ConnectionRecord, error) {
	query := `
		SELECT name, dialect, host, port, user, password, database_name, file_path, ssl_mode, is_active, created_at, updated_at
		FROM connections WHERE is_active = 1 LIMIT 1;
	`
	row := s.db.QueryRowContext(ctx, query)

	var c ConnectionRecord
	var dialectStr string
	var activeInt int
	var createdStr, updatedStr string

	err := row.Scan(&c.Name, &dialectStr, &c.Host, &c.Port, &c.User, &c.Password, &c.Database, &c.FilePath, &c.SSLMode, &activeInt, &createdStr, &updatedStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No active connection set
		}
		return nil, err
	}
	c.Dialect = database.Dialect(dialectStr)
	c.IsActive = true
	return &c, nil
}

// SetActiveConnection marks one connection active and unmarks others
func (s *Storage) SetActiveConnection(ctx context.Context, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE connections SET is_active = 0;"); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, "UPDATE connections SET is_active = 1 WHERE name = ?;", name)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("connection '%s' does not exist", name)
	}

	return tx.Commit()
}

// ListConnections returns all saved connection profiles
func (s *Storage) ListConnections(ctx context.Context) ([]ConnectionRecord, error) {
	query := `
		SELECT name, dialect, host, port, user, database_name, file_path, ssl_mode, is_active, created_at, updated_at
		FROM connections ORDER BY name ASC;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ConnectionRecord
	for rows.Next() {
		var c ConnectionRecord
		var dialectStr string
		var activeInt int
		var createdStr, updatedStr string

		if err := rows.Scan(&c.Name, &dialectStr, &c.Host, &c.Port, &c.User, &c.Database, &c.FilePath, &c.SSLMode, &activeInt, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		c.Dialect = database.Dialect(dialectStr)
		c.IsActive = activeInt == 1
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteConnection removes a connection profile
func (s *Storage) DeleteConnection(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM connections WHERE name = ?;", name)
	return err
}

// RecordQuery saves query metrics into query history
func (s *Storage) RecordQuery(ctx context.Context, rec *QueryHistoryRecord) error {
	query := `
		INSERT INTO query_history (connection_name, query_text, fingerprint, duration_ms, rows_examined, rows_returned, performance_score)
		VALUES (?, ?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.ExecContext(ctx, query,
		rec.ConnectionName, rec.QueryText, rec.Fingerprint, rec.DurationMs,
		rec.RowsExamined, rec.RowsReturned, rec.PerformanceScore,
	)
	return err
}

// GetQueryHistory retrieves recent query records
func (s *Storage) GetQueryHistory(ctx context.Context, limit int) ([]QueryHistoryRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, connection_name, query_text, fingerprint, duration_ms, rows_examined, rows_returned, performance_score, created_at
		FROM query_history ORDER BY id DESC LIMIT ?;
	`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QueryHistoryRecord
	for rows.Next() {
		var r QueryHistoryRecord
		var createdStr string
		if err := rows.Scan(&r.ID, &r.ConnectionName, &r.QueryText, &r.Fingerprint, &r.DurationMs, &r.RowsExamined, &r.RowsReturned, &r.PerformanceScore, &createdStr); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// SaveSnapshot saves a schema snapshot
func (s *Storage) SaveSnapshot(ctx context.Context, name, connName, dbName string, data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize snapshot data: %w", err)
	}

	query := `
		INSERT INTO snapshots (name, connection_name, database_name, data, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			data=excluded.data,
			created_at=CURRENT_TIMESTAMP;
	`
	_, err = s.db.ExecContext(ctx, query, name, connName, dbName, string(bytes))
	return err
}

// GetSnapshot retrieves a snapshot by name
func (s *Storage) GetSnapshot(ctx context.Context, name string) (*SnapshotRecord, error) {
	query := `SELECT id, name, connection_name, database_name, data, created_at FROM snapshots WHERE name = ?;`
	row := s.db.QueryRowContext(ctx, query, name)

	var rec SnapshotRecord
	var createdStr string
	err := row.Scan(&rec.ID, &rec.Name, &rec.ConnectionName, &rec.DatabaseName, &rec.Data, &createdStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("snapshot '%s' not found", name)
		}
		return nil, err
	}
	return &rec, nil
}

// ListSnapshots returns all stored snapshots
func (s *Storage) ListSnapshots(ctx context.Context) ([]SnapshotRecord, error) {
	query := `SELECT id, name, connection_name, database_name, created_at FROM snapshots ORDER BY id DESC;`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []SnapshotRecord
	for rows.Next() {
		var rec SnapshotRecord
		var createdStr string
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.ConnectionName, &rec.DatabaseName, &createdStr); err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteSnapshot deletes a snapshot by name
func (s *Storage) DeleteSnapshot(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM snapshots WHERE name = ?;", name)
	return err
}

// SetSetting saves a persistent key-value configuration
func (s *Storage) SetSetting(ctx context.Context, key, value string) error {
	query := `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value=excluded.value,
			updated_at=CURRENT_TIMESTAMP;
	`
	_, err := s.db.ExecContext(ctx, query, key, value)
	return err
}

// GetSetting reads a persistent configuration value
func (s *Storage) GetSetting(ctx context.Context, key string) (string, error) {
	var val string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?;", key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return val, nil
}
