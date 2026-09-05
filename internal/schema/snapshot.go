package schema

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/storage"
)

// SchemaSnapshot holds complete structural snapshot of a database
type SchemaSnapshot struct {
	Name           string                            `json:"name"`
	DatabaseName   string                            `json:"database_name"`
	Dialect        string                            `json:"dialect"`
	CreatedAt      time.Time                         `json:"created_at"`
	TableCount     int                               `json:"table_count"`
	Tables         map[string]*database.TableDetail `json:"tables"`
}

// SnapshotManager manages creating, retrieving, and comparing snapshots
type SnapshotManager struct {
	store  *storage.Storage
	driver database.Driver
}

func NewSnapshotManager(store *storage.Storage, driver database.Driver) *SnapshotManager {
	return &SnapshotManager{
		store:  store,
		driver: driver,
	}
}

func (s *SnapshotManager) CreateSnapshot(ctx context.Context, db *sql.DB, snapshotName, connName, dbName string) (*SchemaSnapshot, error) {
	details, err := FetchAllTableDetails(ctx, s.driver, db)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch table details for snapshot: %w", err)
	}

	snap := &SchemaSnapshot{
		Name:         snapshotName,
		DatabaseName: dbName,
		Dialect:      string(s.driver.Dialect()),
		CreatedAt:    time.Now(),
		TableCount:   len(details),
		Tables:       details,
	}

	if s.store != nil {
		err = s.store.SaveSnapshot(ctx, snapshotName, connName, dbName, snap)
		if err != nil {
			return nil, fmt.Errorf("failed to save snapshot into local storage: %w", err)
		}
	}

	return snap, nil
}

func (s *SnapshotManager) GetSnapshot(ctx context.Context, snapshotName string) (*SchemaSnapshot, error) {
	if s.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	rec, err := s.store.GetSnapshot(ctx, snapshotName)
	if err != nil {
		return nil, err
	}

	var snap SchemaSnapshot
	if err := json.Unmarshal([]byte(rec.Data), &snap); err != nil {
		return nil, fmt.Errorf("failed to deserialize snapshot: %w", err)
	}

	return &snap, nil
}
