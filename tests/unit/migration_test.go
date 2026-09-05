package unit

import (
	"path/filepath"
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/migration"
)

func TestMigrationAnalyzer(t *testing.T) {
	analyzer := migration.NewAnalyzer()

	// Test non-existent file
	_, err := analyzer.AnalyzeFile("non_existent_file.sql")
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}

	// Test analyzing fixture migration
	fixturePath := filepath.Join("..", "fixtures", "migrations", "v2_migration.sql")
	report, err := analyzer.AnalyzeFile(fixturePath)
	if err != nil {
		t.Fatalf("unexpected error analyzing fixture: %v", err)
	}

	if report.IsSafe {
		t.Errorf("expected migration to be flagged as unsafe due to DROP TABLE/COLUMN")
	}

	if report.TotalRisks == 0 {
		t.Errorf("expected detected risks in v2_migration.sql, got 0")
	}
}
