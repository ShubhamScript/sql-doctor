package schema

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
)

// SchemaIssue specifies a schema health problem
type SchemaIssue struct {
	Level       string `json:"level"` // CRITICAL, WARNING, INFO
	TableName   string `json:"table_name"`
	Issue       string `json:"issue"`
	Description string `json:"description"`
	Remedy      string `json:"remedy"`
}

// SchemaReport summarizes the overall health of a database schema
type SchemaReport struct {
	TotalTables      int           `json:"total_tables"`
	TotalIndexes     int           `json:"total_indexes"`
	TotalForeignKeys int           `json:"total_foreign_keys"`
	Issues           []SchemaIssue `json:"issues"`
	HealthScore      int           `json:"health_score"`
}

// SchemaAnalyzer analyzes overall database schema quality and anti-patterns
type SchemaAnalyzer struct {
	driver database.Driver
}

func NewSchemaAnalyzer(driver database.Driver) *SchemaAnalyzer {
	return &SchemaAnalyzer{driver: driver}
}

func (s *SchemaAnalyzer) Analyze(ctx context.Context, db *sql.DB) (*SchemaReport, error) {
	tables, err := s.driver.Tables(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tables: %w", err)
	}

	report := &SchemaReport{
		TotalTables: len(tables),
		HealthScore: 100,
	}

	for _, t := range tables {
		if t.Type == "VIEW" {
			continue
		}

		detail, err := s.driver.DescribeTable(ctx, db, t.Name)
		if err != nil {
			continue
		}

		report.TotalIndexes += len(detail.Indexes)
		report.TotalForeignKeys += len(detail.ForeignKeys)

		// 1. Missing Primary Key
		hasPK := false
		for _, col := range detail.Columns {
			if col.IsPrimaryKey {
				hasPK = true
				break
			}
		}
		if !hasPK {
			report.Issues = append(report.Issues, SchemaIssue{
				Level:       "CRITICAL",
				TableName:   t.Name,
				Issue:       "Missing Primary Key",
				Description: fmt.Sprintf("Table '%s' does not have a defined Primary Key.", t.Name),
				Remedy:      "Add an explicit PRIMARY KEY (e.g. auto-incrementing ID or UUID) to identify records uniquely and optimize clustered index lookups.",
			})
			report.HealthScore -= 20
		}

		// 2. Unindexed Foreign Keys
		for _, fk := range detail.ForeignKeys {
			if len(fk.Columns) == 0 {
				continue
			}
			fkLeadCol := fk.Columns[0]
			indexed := false
			for _, idx := range detail.Indexes {
				if len(idx.Columns) > 0 && strings.EqualFold(idx.Columns[0], fkLeadCol) {
					indexed = true
					break
				}
			}
			if !indexed {
				report.Issues = append(report.Issues, SchemaIssue{
					Level:       "WARNING",
					TableName:   t.Name,
					Issue:       "Unindexed Foreign Key",
					Description: fmt.Sprintf("Foreign key '%s' on column '%s' references '%s' but lacks a covering index.", fk.Name, fkLeadCol, fk.RefTableName),
					Remedy:      fmt.Sprintf("CREATE INDEX idx_%s_%s ON %s (%s); — Prevents table-level lock escalation during parent table updates/deletions.", t.Name, fkLeadCol, t.Name, fkLeadCol),
				})
				report.HealthScore -= 10
			}
		}

		// 3. Redundant / Duplicate Indexes
		for i := 0; i < len(detail.Indexes); i++ {
			for j := 0; j < len(detail.Indexes); j++ {
				if i == j {
					continue
				}
				idxA := detail.Indexes[i]
				idxB := detail.Indexes[j]
				// If idxA's columns are an exact prefix of idxB's columns and idxA is not unique/primary
				if !idxA.IsPrimary && !idxA.IsUnique && len(idxA.Columns) < len(idxB.Columns) {
					isPrefix := true
					for k := 0; k < len(idxA.Columns); k++ {
						if !strings.EqualFold(idxA.Columns[k], idxB.Columns[k]) {
							isPrefix = false
							break
						}
					}
					if isPrefix {
						report.Issues = append(report.Issues, SchemaIssue{
							Level:       "WARNING",
							TableName:   t.Name,
							Issue:       "Redundant Prefix Index",
							Description: fmt.Sprintf("Index '%s' (%s) is a redundant left-prefix of composite index '%s' (%s).", idxA.Name, strings.Join(idxA.Columns, ", "), idxB.Name, strings.Join(idxB.Columns, ", ")),
							Remedy:      fmt.Sprintf("DROP INDEX %s — Index '%s' already satisfies queries filtering on (%s).", idxA.Name, idxB.Name, strings.Join(idxA.Columns, ", ")),
						})
						report.HealthScore -= 5
					}
				}
			}
		}

		// 4. Overly Wide Tables (> 35 columns)
		if len(detail.Columns) > 35 {
			report.Issues = append(report.Issues, SchemaIssue{
				Level:       "INFO",
				TableName:   t.Name,
				Issue:       "Overly Wide Table",
				Description: fmt.Sprintf("Table '%s' contains %d columns.", t.Name, len(detail.Columns)),
				Remedy:      "Consider normalizing or vertically partitioning rarely-accessed columns into satellite tables.",
			})
			report.HealthScore -= 5
		}
	}

	if report.HealthScore < 0 {
		report.HealthScore = 0
	}

	return report, nil
}
