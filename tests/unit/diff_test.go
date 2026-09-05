package unit

import (
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/schema"
)

func TestSchemaDiff(t *testing.T) {
	src := map[string]*database.TableDetail{
		"users": {
			Table: database.TableInfo{Name: "users"},
			Columns: []database.ColumnInfo{
				{Name: "id", RawType: "INTEGER", IsPrimaryKey: true},
				{Name: "email", RawType: "VARCHAR(255)", DataType: "VARCHAR"},
				{Name: "status", RawType: "VARCHAR(50)", DataType: "VARCHAR"},
			},
		},
	}

	tgt := map[string]*database.TableDetail{
		"users": {
			Table: database.TableInfo{Name: "users"},
			Columns: []database.ColumnInfo{
				{Name: "id", RawType: "INTEGER", IsPrimaryKey: true},
				{Name: "email", RawType: "TEXT", DataType: "TEXT"}, // Mismatched type
			},
		},
	}

	diffEngine := schema.NewDiffEngine()
	diff := diffEngine.Compare(src, tgt)

	if diff.TotalDiffs != 2 {
		t.Fatalf("expected 2 differences (status added, email modified), got %d", diff.TotalDiffs)
	}

	hasAddedStatus := false
	hasModifiedEmail := false
	for _, item := range diff.Differences {
		if item.ObjectName == "users.status" && item.Action == "ADDED" {
			hasAddedStatus = true
		}
		if item.ObjectName == "users.email" && item.Action == "MODIFIED" {
			hasModifiedEmail = true
		}
	}

	if !hasAddedStatus || !hasModifiedEmail {
		t.Errorf("expected added status and modified email in diff, got: %+v", diff.Differences)
	}
}
