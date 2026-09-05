package schema

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
)

// DiffItem represents a single schema delta
type DiffItem struct {
	Type        string `json:"type"` // TABLE, COLUMN, INDEX, FOREIGN_KEY
	Action      string `json:"action"` // ADDED, REMOVED, MODIFIED
	ObjectName  string `json:"object_name"`
	Details     string `json:"details"`
	MigrationSQL string `json:"migration_sql,omitempty"`
}

// DiffResult aggregates differences between two schemas
type DiffResult struct {
	SourceSchema string     `json:"source_schema"`
	TargetSchema string     `json:"target_schema"`
	Differences  []DiffItem `json:"differences"`
	TotalDiffs   int        `json:"total_differences"`
}

// DiffEngine compares two database schemas
type DiffEngine struct{}

func NewDiffEngine() *DiffEngine {
	return &DiffEngine{}
}

// CompareDatabaseDetails compares two sets of table details
func (d *DiffEngine) Compare(sourceDetails, targetDetails map[string]*database.TableDetail) *DiffResult {
	res := &DiffResult{
		Differences: []DiffItem{},
	}

	// 1. Missing or Extra Tables
	for srcName, srcDetail := range sourceDetails {
		targetDetail, exists := targetDetails[srcName]
		if !exists {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "TABLE",
				Action:       "ADDED",
				ObjectName:   srcName,
				Details:      fmt.Sprintf("Table '%s' exists in source but is missing in target.", srcName),
				MigrationSQL: generateCreateTableSQL(srcDetail),
			})
			continue
		}

		// 2. Compare Columns
		d.compareColumns(srcDetail, targetDetail, res)

		// 3. Compare Indexes
		d.compareIndexes(srcDetail, targetDetail, res)

		// 4. Compare Foreign Keys
		d.compareForeignKeys(srcDetail, targetDetail, res)
	}

	for tgtName := range targetDetails {
		if _, exists := sourceDetails[tgtName]; !exists {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "TABLE",
				Action:       "REMOVED",
				ObjectName:   tgtName,
				Details:      fmt.Sprintf("Table '%s' exists in target but was removed in source.", tgtName),
				MigrationSQL: fmt.Sprintf("DROP TABLE IF EXISTS %s;", tgtName),
			})
		}
	}

	res.TotalDiffs = len(res.Differences)
	return res
}

func (d *DiffEngine) compareColumns(src, tgt *database.TableDetail, res *DiffResult) {
	srcColMap := make(map[string]database.ColumnInfo)
	for _, c := range src.Columns {
		srcColMap[strings.ToLower(c.Name)] = c
	}
	tgtColMap := make(map[string]database.ColumnInfo)
	for _, c := range tgt.Columns {
		tgtColMap[strings.ToLower(c.Name)] = c
	}

	for name, srcCol := range srcColMap {
		tgtCol, exists := tgtColMap[name]
		if !exists {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "COLUMN",
				Action:       "ADDED",
				ObjectName:   fmt.Sprintf("%s.%s", src.Table.Name, srcCol.Name),
				Details:      fmt.Sprintf("Column '%s' (%s) missing in target table.", srcCol.Name, srcCol.RawType),
				MigrationSQL: fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", src.Table.Name, srcCol.Name, srcCol.RawType),
			})
			continue
		}

		// Check type / nullability
		if !strings.EqualFold(srcCol.DataType, tgtCol.DataType) {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "COLUMN",
				Action:       "MODIFIED",
				ObjectName:   fmt.Sprintf("%s.%s", src.Table.Name, srcCol.Name),
				Details:      fmt.Sprintf("Data type mismatch: source has '%s', target has '%s'.", srcCol.RawType, tgtCol.RawType),
				MigrationSQL: fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s;", src.Table.Name, srcCol.Name, srcCol.RawType),
			})
		}
		if srcCol.IsNullable != tgtCol.IsNullable {
			nullStr := "NOT NULL"
			if srcCol.IsNullable {
				nullStr = "NULL"
			}
			res.Differences = append(res.Differences, DiffItem{
				Type:       "COLUMN",
				Action:     "MODIFIED",
				ObjectName: fmt.Sprintf("%s.%s", src.Table.Name, srcCol.Name),
				Details:    fmt.Sprintf("Nullability mismatch: source is %s, target is %v.", nullStr, tgtCol.IsNullable),
			})
		}
	}

	for name, tgtCol := range tgtColMap {
		if _, exists := srcColMap[name]; !exists {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "COLUMN",
				Action:       "REMOVED",
				ObjectName:   fmt.Sprintf("%s.%s", tgt.Table.Name, tgtCol.Name),
				Details:      fmt.Sprintf("Column '%s' exists in target but was removed in source.", tgtCol.Name),
				MigrationSQL: fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", tgt.Table.Name, tgtCol.Name),
			})
		}
	}
}

func (d *DiffEngine) compareIndexes(src, tgt *database.TableDetail, res *DiffResult) {
	srcIdxMap := make(map[string]database.IndexInfo)
	for _, idx := range src.Indexes {
		srcIdxMap[strings.ToLower(idx.Name)] = idx
	}
	tgtIdxMap := make(map[string]database.IndexInfo)
	for _, idx := range tgt.Indexes {
		tgtIdxMap[strings.ToLower(idx.Name)] = idx
	}

	for name, srcIdx := range srcIdxMap {
		if _, exists := tgtIdxMap[name]; !exists && !srcIdx.IsPrimary {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "INDEX",
				Action:       "ADDED",
				ObjectName:   fmt.Sprintf("%s.%s", src.Table.Name, srcIdx.Name),
				Details:      fmt.Sprintf("Index '%s' on (%s) missing in target.", srcIdx.Name, strings.Join(srcIdx.Columns, ", ")),
				MigrationSQL: fmt.Sprintf("CREATE INDEX %s ON %s (%s);", srcIdx.Name, src.Table.Name, strings.Join(srcIdx.Columns, ", ")),
			})
		}
	}

	for name, tgtIdx := range tgtIdxMap {
		if _, exists := srcIdxMap[name]; !exists && !tgtIdx.IsPrimary {
			res.Differences = append(res.Differences, DiffItem{
				Type:         "INDEX",
				Action:       "REMOVED",
				ObjectName:   fmt.Sprintf("%s.%s", tgt.Table.Name, tgtIdx.Name),
				Details:      fmt.Sprintf("Index '%s' exists in target but was removed in source.", tgtIdx.Name),
				MigrationSQL: fmt.Sprintf("DROP INDEX %s ON %s;", tgtIdx.Name, tgt.Table.Name),
			})
		}
	}
}

func (d *DiffEngine) compareForeignKeys(src, tgt *database.TableDetail, res *DiffResult) {
	srcFkMap := make(map[string]database.ForeignKeyInfo)
	for _, fk := range src.ForeignKeys {
		srcFkMap[strings.ToLower(fk.Name)] = fk
	}
	tgtFkMap := make(map[string]database.ForeignKeyInfo)
	for _, fk := range tgt.ForeignKeys {
		tgtFkMap[strings.ToLower(fk.Name)] = fk
	}

	for name, srcFk := range srcFkMap {
		if _, exists := tgtFkMap[name]; !exists {
			res.Differences = append(res.Differences, DiffItem{
				Type:       "FOREIGN_KEY",
				Action:     "ADDED",
				ObjectName: fmt.Sprintf("%s.%s", src.Table.Name, srcFk.Name),
				Details:    fmt.Sprintf("Foreign key '%s' (%s -> %s.%s) missing in target.", srcFk.Name, strings.Join(srcFk.Columns, ", "), srcFk.RefTableName, strings.Join(srcFk.RefColumns, ", ")),
			})
		}
	}
}

func generateCreateTableSQL(detail *database.TableDetail) string {
	var cols []string
	for _, c := range detail.Columns {
		def := fmt.Sprintf("  %s %s", c.Name, c.RawType)
		if !c.IsNullable {
			def += " NOT NULL"
		}
		if c.IsPrimaryKey {
			def += " PRIMARY KEY"
		}
		cols = append(cols, def)
	}
	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);", detail.Table.Name, strings.Join(cols, ",\n"))
}

// FetchAllTableDetails fetches all table metadata for a database connection
func FetchAllTableDetails(ctx context.Context, driver database.Driver, db *sql.DB) (map[string]*database.TableDetail, error) {
	tables, err := driver.Tables(ctx, db)
	if err != nil {
		return nil, err
	}

	details := make(map[string]*database.TableDetail)
	for _, t := range tables {
		detail, err := driver.DescribeTable(ctx, db, t.Name)
		if err == nil && detail != nil {
			details[t.Name] = detail
		}
	}
	return details, nil
}
