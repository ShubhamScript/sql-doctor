package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sql-doctor/sql-doctor/internal/database"
	_ "modernc.org/sqlite"
)

type Driver struct{}

func New() database.Driver {
	return &Driver{}
}

func (d *Driver) Dialect() database.Dialect {
	return database.DialectSQLite
}

func (d *Driver) DSN(cfg *database.ConnectionConfig) string {
	if cfg.FilePath != "" {
		return cfg.FilePath
	}
	if cfg.Database != "" {
		return cfg.Database
	}
	return ":memory:"
}

func (d *Driver) Connect(ctx context.Context, cfg *database.ConnectionConfig) (*sql.DB, error) {
	dsn := d.DSN(cfg)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(tCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

func (d *Driver) Version(ctx context.Context, db *sql.DB) (string, error) {
	var ver string
	err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&ver)
	if err != nil {
		return "", err
	}
	return "SQLite " + ver, nil
}

func (d *Driver) Tables(ctx context.Context, db *sql.DB) ([]database.TableInfo, error) {
	query := `
		SELECT name, type 
		FROM sqlite_master 
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name ASC;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []database.TableInfo
	for rows.Next() {
		var name, tblType string
		if err := rows.Scan(&name, &tblType); err != nil {
			return nil, err
		}

		tblTypeUpper := "BASE TABLE"
		if strings.ToLower(tblType) == "view" {
			tblTypeUpper = "VIEW"
		}

		// Estimate row count
		var count int64
		// Best effort row count
		_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(1) FROM \"%s\"", name)).Scan(&count)

		tables = append(tables, database.TableInfo{
			Name:     name,
			Type:     tblTypeUpper,
			RowCount: count,
			Engine:   "SQLite",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tables, nil
}

func (d *Driver) DescribeTable(ctx context.Context, db *sql.DB, table string) (*database.TableDetail, error) {
	// 1. Get Columns via PRAGMA table_info
	colQuery := fmt.Sprintf("PRAGMA table_info(\"%s\")", table)
	rows, err := db.QueryContext(ctx, colQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table columns: %w", err)
	}
	defer rows.Close()

	var columns []database.ColumnInfo
	var pkCols []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString

		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}

		isPK := pk > 0
		if isPK {
			pkCols = append(pkCols, name)
		}

		var defaultVal *string
		if dfltValue.Valid {
			v := dfltValue.String
			defaultVal = &v
		}

		columns = append(columns, database.ColumnInfo{
			Name:         name,
			Position:     cid + 1,
			DataType:     strings.ToUpper(colType),
			RawType:      colType,
			IsNullable:   notNull == 0,
			DefaultValue: defaultVal,
			IsPrimaryKey: isPK,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Get Indexes
	indexes, err := d.Indexes(ctx, db, table)
	if err != nil {
		return nil, err
	}

	// 3. Get Foreign Keys
	fks, err := d.ForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}

	// 4. Mark FK and Unique flags on columns
	fkCols := make(map[string]bool)
	for _, fk := range fks {
		for _, c := range fk.Columns {
			fkCols[c] = true
		}
	}
	uniqueCols := make(map[string]bool)
	for _, idx := range indexes {
		if idx.IsUnique && len(idx.Columns) == 1 {
			uniqueCols[idx.Columns[0]] = true
		}
	}

	for i := range columns {
		if fkCols[columns[i].Name] {
			columns[i].IsForeignKey = true
		}
		if uniqueCols[columns[i].Name] {
			columns[i].IsUnique = true
		}
	}

	var rowCount int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(1) FROM \"%s\"", table)).Scan(&rowCount)

	return &database.TableDetail{
		Table: database.TableInfo{
			Name:       table,
			Type:       "BASE TABLE",
			RowCount:   rowCount,
			PrimaryKey: strings.Join(pkCols, ", "),
			Engine:     "SQLite",
		},
		Columns:     columns,
		Indexes:     indexes,
		ForeignKeys: fks,
	}, nil
}

func (d *Driver) Indexes(ctx context.Context, db *sql.DB, table string) ([]database.IndexInfo, error) {
	idxQuery := fmt.Sprintf("PRAGMA index_list(\"%s\")", table)
	rows, err := db.QueryContext(ctx, idxQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []database.IndexInfo
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int

		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}

		// Get index columns
		colRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(\"%s\")", name))
		if err != nil {
			continue
		}

		var cols []string
		for colRows.Next() {
			var seqno, cid int
			var colName string
			if err := colRows.Scan(&seqno, &cid, &colName); err == nil {
				cols = append(cols, colName)
			}
		}
		if err := colRows.Err(); err != nil {
			colRows.Close()
			return nil, err
		}
		colRows.Close()

		isPK := origin == "pk"
		indexes = append(indexes, database.IndexInfo{
			Name:      name,
			TableName: table,
			Columns:   cols,
			IsPrimary: isPK,
			IsUnique:  unique == 1,
			Type:      "BTREE",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return indexes, nil
}

func (d *Driver) ForeignKeys(ctx context.Context, db *sql.DB, table string) ([]database.ForeignKeyInfo, error) {
	query := fmt.Sprintf("PRAGMA foreign_key_list(\"%s\")", table)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fkMap := make(map[int]*database.ForeignKeyInfo)
	for rows.Next() {
		var id, seq int
		var refTable, fromCol, toCol, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}

		fk, exists := fkMap[id]
		if !exists {
			fk = &database.ForeignKeyInfo{
				Name:         fmt.Sprintf("fk_%s_%s_%d", table, refTable, id),
				TableName:    table,
				RefTableName: refTable,
				OnUpdate:     onUpdate,
				OnDelete:     onDelete,
			}
			fkMap[id] = fk
		}
		fk.Columns = append(fk.Columns, fromCol)
		fk.RefColumns = append(fk.RefColumns, toCol)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []database.ForeignKeyInfo
	for _, fk := range fkMap {
		result = append(result, *fk)
	}
	return result, nil
}

func (d *Driver) Relationships(ctx context.Context, db *sql.DB) ([]database.RelationshipInfo, error) {
	tables, err := d.Tables(ctx, db)
	if err != nil {
		return nil, err
	}

	var relationships []database.RelationshipInfo
	knownTables := make(map[string]bool)
	for _, t := range tables {
		knownTables[strings.ToLower(t.Name)] = true
	}

	// 1. Explicit foreign keys
	for _, t := range tables {
		fks, err := d.ForeignKeys(ctx, db, t.Name)
		if err != nil {
			continue
		}
		for _, fk := range fks {
			// Check for orphan records
			var orphanCount int64
			if len(fk.Columns) > 0 && len(fk.RefColumns) > 0 {
				orphanQ := fmt.Sprintf(`
					SELECT COUNT(1) 
					FROM "%s" AS a 
					LEFT JOIN "%s" AS b ON a."%s" = b."%s" 
					WHERE a."%s" IS NOT NULL AND b."%s" IS NULL;
				`, fk.TableName, fk.RefTableName, fk.Columns[0], fk.RefColumns[0], fk.Columns[0], fk.RefColumns[0])
				_ = db.QueryRowContext(ctx, orphanQ).Scan(&orphanCount)
			}

			relationships = append(relationships, database.RelationshipInfo{
				FromTable:   fk.TableName,
				FromColumns: fk.Columns,
				ToTable:     fk.RefTableName,
				ToColumns:   fk.RefColumns,
				IsExplicit:  true,
				Constraint:  fk.Name,
				OrphanCount: orphanCount,
			})
		}

		// 2. Inferred relationships (naming convention e.g., author_id -> authors.id or author.id)
		detail, err := d.DescribeTable(ctx, db, t.Name)
		if err != nil {
			continue
		}

		for _, col := range detail.Columns {
			if col.IsForeignKey || col.IsPrimaryKey {
				continue
			}
			colLower := strings.ToLower(col.Name)
			if strings.HasSuffix(colLower, "_id") {
				targetPrefix := strings.TrimSuffix(colLower, "_id")
				var targetTable string
				if knownTables[targetPrefix] {
					targetTable = targetPrefix
				} else if knownTables[targetPrefix+"s"] {
					targetTable = targetPrefix + "s"
				} else if knownTables[targetPrefix+"es"] {
					targetTable = targetPrefix + "es"
				}

				if targetTable != "" && !strings.EqualFold(targetTable, t.Name) {
					// Check if already covered by explicit FK
					alreadyCovered := false
					for _, r := range relationships {
						if strings.EqualFold(r.FromTable, t.Name) && len(r.FromColumns) > 0 && strings.EqualFold(r.FromColumns[0], col.Name) {
							alreadyCovered = true
							break
						}
					}
					if !alreadyCovered {
						var orphanCount int64
						orphanQ := fmt.Sprintf(`
							SELECT COUNT(1) 
							FROM "%s" AS a 
							LEFT JOIN "%s" AS b ON a."%s" = b."id" 
							WHERE a."%s" IS NOT NULL AND b."id" IS NULL;
						`, t.Name, targetTable, col.Name, col.Name)
						_ = db.QueryRowContext(ctx, orphanQ).Scan(&orphanCount)

						relationships = append(relationships, database.RelationshipInfo{
							FromTable:   t.Name,
							FromColumns: []string{col.Name},
							ToTable:     targetTable,
							ToColumns:   []string{"id"},
							IsExplicit:  false,
							OrphanCount: orphanCount,
						})
					}
				}
			}
		}
	}

	return relationships, nil
}

func (d *Driver) Explain(ctx context.Context, db *sql.DB, query string, analyze bool) (*database.ExplainResult, error) {
	explainCmd := "EXPLAIN QUERY PLAN " + query
	rows, err := db.QueryContext(ctx, explainCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to run EXPLAIN QUERY PLAN: %w", err)
	}
	defer rows.Close()

	var rawLines []string
	var planNodes []*database.PlanNode
	hasFullScan := false
	var fullScanTables []string

	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			return nil, err
		}
		rawLines = append(rawLines, fmt.Sprintf("%d | %d | %s", id, parent, detail))

		isScan := strings.Contains(strings.ToUpper(detail), "SCAN TABLE") || strings.Contains(strings.ToUpper(detail), "SCAN")
		isIndex := strings.Contains(strings.ToUpper(detail), "USING INDEX") || strings.Contains(strings.ToUpper(detail), "SEARCH TABLE")

		scanType := "Index Scan"
		if isScan && !isIndex {
			scanType = "ALL (Full Table Scan)"
			hasFullScan = true
			// extract table name
			parts := strings.Fields(detail)
			for i, p := range parts {
				if strings.ToUpper(p) == "TABLE" && i+1 < len(parts) {
					fullScanTables = append(fullScanTables, parts[i+1])
				}
			}
		}

		node := &database.PlanNode{
			Operation: detail,
			ScanType:  scanType,
			Extra:     detail,
		}
		planNodes = append(planNodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rootNode := &database.PlanNode{
		Operation: "Query Plan",
		Children:  planNodes,
	}

	return &database.ExplainResult{
		Dialect:      database.DialectSQLite,
		RawOutput:    strings.Join(rawLines, "\n"),
		RootNode:     rootNode,
		HasFullScan:  hasFullScan,
		FullScanTabs: fullScanTables,
		UsesFilesort: strings.Contains(strings.ToUpper(strings.Join(rawLines, " ")), "USE TEMP B-TREE FOR ORDER BY"),
		UsesTemp:     strings.Contains(strings.ToUpper(strings.Join(rawLines, " ")), "USE TEMP B-TREE"),
	}, nil
}

func (d *Driver) TableStats(ctx context.Context, db *sql.DB, table string) (*database.TableStats, error) {
	var count int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(1) FROM \"%s\"", table)).Scan(&count)

	return &database.TableStats{
		TableName:     table,
		EstimatedRows: count,
		TotalBytes:    count * 128, // Approximation for SQLite
		DataBytes:     count * 100,
		IndexBytes:    count * 28,
	}, nil
}

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	uuidRegex  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T|\s)\d{2}:\d{2}:\d{2}`)
)

func (d *Driver) SampleColumnData(ctx context.Context, db *sql.DB, table, col string, limit int) (*database.ColumnSampleStats, error) {
	if limit <= 0 {
		limit = 1000
	}

	query := fmt.Sprintf("SELECT \"%s\" FROM \"%s\" LIMIT %d", col, table, limit)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to sample column %s: %w", col, err)
	}
	defer rows.Close()

	stats := &database.ColumnSampleStats{
		ColumnName: col,
		MinLength:  999999,
		MaxLength:  0,
		IsNumeric:  true,
		IsBoolean:  true,
		IsDateTime: true,
		IsUUID:     true,
		IsEmail:    true,
		IsJSON:     true,
	}

	distinctSet := make(map[string]bool)
	var totalLen int64

	for rows.Next() {
		stats.TotalSampled++
		var val sql.NullString
		if err := rows.Scan(&val); err != nil {
			continue
		}

		if !val.Valid {
			stats.NullCount++
			continue
		}

		str := val.String
		l := len(str)
		totalLen += int64(l)
		if l < stats.MinLength {
			stats.MinLength = l
		}
		if l > stats.MaxLength {
			stats.MaxLength = l
		}
		if stats.MinValue == "" || str < stats.MinValue {
			stats.MinValue = str
		}
		if stats.MaxValue == "" || str > stats.MaxValue {
			stats.MaxValue = str
		}

		distinctSet[str] = true
		if len(stats.SampleValues) < 5 {
			stats.SampleValues = append(stats.SampleValues, str)
		}

		// Heuristic pattern checks
		strTrim := strings.TrimSpace(str)
		if stats.IsNumeric && !isNumericString(strTrim) {
			stats.IsNumeric = false
		}
		if stats.IsBoolean && !isBooleanString(strTrim) {
			stats.IsBoolean = false
		}
		if stats.IsDateTime && !dateRegex.MatchString(strTrim) {
			stats.IsDateTime = false
		}
		if stats.IsUUID && !uuidRegex.MatchString(strTrim) {
			stats.IsUUID = false
		}
		if stats.IsEmail && !emailRegex.MatchString(strTrim) {
			stats.IsEmail = false
		}
		if stats.IsJSON && !(strings.HasPrefix(strTrim, "{") && strings.HasSuffix(strTrim, "}")) &&
			!(strings.HasPrefix(strTrim, "[") && strings.HasSuffix(strTrim, "]")) {
			stats.IsJSON = false
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if stats.MinLength == 999999 {
		stats.MinLength = 0
	}
	stats.DistinctCount = int64(len(distinctSet))
	nonNull := stats.TotalSampled - stats.NullCount
	if stats.TotalSampled > 0 {
		stats.NullPercentage = (float64(stats.NullCount) / float64(stats.TotalSampled)) * 100.0
	}
	if nonNull > 0 {
		stats.AvgLength = float64(totalLen) / float64(nonNull)
	} else {
		stats.IsNumeric = false
		stats.IsBoolean = false
		stats.IsDateTime = false
		stats.IsUUID = false
		stats.IsEmail = false
		stats.IsJSON = false
	}

	return stats, nil
}

func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	dotCount := 0
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r == '.' {
			dotCount++
			if dotCount > 1 {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isBooleanString(s string) bool {
	lower := strings.ToLower(s)
	return lower == "true" || lower == "false" || lower == "1" || lower == "0" || lower == "t" || lower == "f" || lower == "yes" || lower == "no"
}
