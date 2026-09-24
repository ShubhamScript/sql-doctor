package unit

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/cli"
	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/database/sqlite"
	"github.com/sql-doctor/sql-doctor/internal/storage"
)

func TestStorageSessionConnection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sql-doctor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	store, err := storage.OpenStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// 1. Initial state - no session
	sess, err := store.GetSessionConnection(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess != nil {
		t.Fatalf("expected nil session, got %+v", sess)
	}

	// 2. Save session connection without a database selected
	rec := &storage.ConnectionRecord{
		Name:     "default",
		Dialect:  database.DialectMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Database: "",
		IsActive: true,
	}
	if err := store.SaveSessionConnection(ctx, rec); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// 3. Retrieve session connection
	sess, err = store.GetSessionConnection(ctx)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if sess == nil || sess.Host != "127.0.0.1" || sess.Database != "" {
		t.Fatalf("unexpected session record: %+v", sess)
	}

	// 4. Update database on session connection
	updated, err := store.UpdateConnectionDatabase(ctx, "", "ecommerce_prod")
	if err != nil {
		t.Fatalf("failed to update session database: %v", err)
	}
	if updated.Database != "ecommerce_prod" {
		t.Errorf("expected updated database 'ecommerce_prod', got '%s'", updated.Database)
	}

	// Verify persistence in session
	sess, err = store.GetSessionConnection(ctx)
	if err != nil || sess == nil || sess.Database != "ecommerce_prod" {
		t.Fatalf("expected session database 'ecommerce_prod', got: %+v", sess)
	}

	// 5. Clear session
	if err := store.ClearSessionConnection(ctx); err != nil {
		t.Fatalf("failed to clear session: %v", err)
	}
	sess, err = store.GetSessionConnection(ctx)
	if err != nil || sess != nil {
		t.Fatalf("expected cleared session, got: %+v", sess)
	}
}

func TestStorageUpdateNamedConnectionDatabase(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sql-doctor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	store, err := storage.OpenStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Save a named connection profile without selecting a database
	rec := &storage.ConnectionRecord{
		Name:     "my-mysql",
		Dialect:  database.DialectMySQL,
		Host:     "localhost",
		Port:     3306,
		User:     "admin",
		Database: "",
		IsActive: true,
	}
	if err := store.SaveConnection(ctx, rec); err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}

	// Later select a database for this connection
	updated, err := store.UpdateConnectionDatabase(ctx, "my-mysql", "analytics")
	if err != nil {
		t.Fatalf("failed to update connection database: %v", err)
	}
	if updated.Database != "analytics" {
		t.Errorf("expected database 'analytics', got '%s'", updated.Database)
	}

	// Check fetched connection has updated database
	fetched, err := store.GetConnection(ctx, "my-mysql")
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}
	if fetched.Database != "analytics" {
		t.Errorf("expected fetched database 'analytics', got '%s'", fetched.Database)
	}
}

func TestSQLiteDatabases(t *testing.T) {
	ctx := context.Background()
	drv := sqlite.New()

	cfg := &database.ConnectionConfig{
		Dialect:  database.DialectSQLite,
		FilePath: ":memory:",
	}
	db, err := drv.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect sqlite: %v", err)
	}
	defer db.Close()

	dbs, err := drv.Databases(ctx, db)
	if err != nil {
		t.Fatalf("failed to list databases: %v", err)
	}
	if len(dbs) == 0 || dbs[0] != "main" {
		t.Errorf("expected ['main'], got %v", dbs)
	}
}

// mockDriver implements database.Driver for testing EnsureDatabase
type mockDriver struct {
	availableDBs []string
}

func (m *mockDriver) Dialect() database.Dialect                               { return database.DialectMySQL }
func (m *mockDriver) DSN(cfg *database.ConnectionConfig) string              { return "" }
func (m *mockDriver) Connect(ctx context.Context, cfg *database.ConnectionConfig) (*sql.DB, error) {
	return nil, nil
}
func (m *mockDriver) Ping(ctx context.Context, db *sql.DB) error             { return nil }
func (m *mockDriver) Version(ctx context.Context, db *sql.DB) (string, error) {
	return "MySQL 8.0.36", nil
}
func (m *mockDriver) Databases(ctx context.Context, db *sql.DB) ([]string, error) {
	return m.availableDBs, nil
}
func (m *mockDriver) Tables(ctx context.Context, db *sql.DB) ([]database.TableInfo, error) {
	return nil, nil
}
func (m *mockDriver) DescribeTable(ctx context.Context, db *sql.DB, t string) (*database.TableDetail, error) {
	return nil, nil
}
func (m *mockDriver) Indexes(ctx context.Context, db *sql.DB, t string) ([]database.IndexInfo, error) {
	return nil, nil
}
func (m *mockDriver) ForeignKeys(ctx context.Context, db *sql.DB, t string) ([]database.ForeignKeyInfo, error) {
	return nil, nil
}
func (m *mockDriver) Relationships(ctx context.Context, db *sql.DB) ([]database.RelationshipInfo, error) {
	return nil, nil
}
func (m *mockDriver) Explain(ctx context.Context, db *sql.DB, q string, a bool) (*database.ExplainResult, error) {
	return nil, nil
}
func (m *mockDriver) TableStats(ctx context.Context, db *sql.DB, t string) (*database.TableStats, error) {
	return nil, nil
}
func (m *mockDriver) SampleColumnData(ctx context.Context, db *sql.DB, t, c string, l int) (*database.ColumnSampleStats, error) {
	return nil, nil
}

func TestEnsureDatabase(t *testing.T) {
	ctx := context.Background()

	mock := &mockDriver{
		availableDBs: []string{"shop_db", "inventory", "test_db"},
	}

	// Case 1: Database is empty for MySQL -> should return error with suggestions
	cfgNoDB := &database.ConnectionConfig{
		Name:     "dev-server",
		Dialect:  database.DialectMySQL,
		Database: "",
	}
	err := cli.EnsureDatabase(ctx, nil, mock, cfgNoDB)
	if err == nil {
		t.Fatalf("expected error when database is empty")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "no database selected for connection 'dev-server'") {
		t.Errorf("expected error message to mention missing database, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "sql-doctor use <database-name>") {
		t.Errorf("expected suggestion to run 'sql-doctor use <database-name>', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "shop_db") || !strings.Contains(errMsg, "inventory") {
		t.Errorf("expected available databases listed in suggestion, got: %s", errMsg)
	}

	// Case 2: Database is specified -> should succeed without error
	cfgWithDB := &database.ConnectionConfig{
		Name:     "dev-server",
		Dialect:  database.DialectMySQL,
		Database: "shop_db",
	}
	if err := cli.EnsureDatabase(ctx, nil, mock, cfgWithDB); err != nil {
		t.Errorf("expected nil error when database is present, got: %v", err)
	}

	// Case 3: SQLite dialect -> should succeed even without database name
	cfgSQLite := &database.ConnectionConfig{
		Name:     "local-sqlite",
		Dialect:  database.DialectSQLite,
		FilePath: "test.db",
	}
	if err := cli.EnsureDatabase(ctx, nil, mock, cfgSQLite); err != nil {
		t.Errorf("expected nil error for sqlite, got: %v", err)
	}

	// Case 4: Inside Shell -> should suggest 'use <database-name>' without sql-doctor prefix
	shellErr := cli.EnsureShellDatabase(ctx, nil, mock, cfgNoDB)
	if shellErr == nil {
		t.Fatalf("expected error for shell without database")
	}
	shellErrMsg := shellErr.Error()
	if !strings.Contains(shellErrMsg, "  use <database-name>\n") {
		t.Errorf("expected shell suggestion to contain 'use <database-name>', got: %s", shellErrMsg)
	}
	if strings.Contains(shellErrMsg, "sql-doctor use") {
		t.Errorf("shell suggestion should NOT contain 'sql-doctor use', got: %s", shellErrMsg)
	}
}
