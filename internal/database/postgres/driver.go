package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/sql-doctor/sql-doctor/internal/database"
)

type Driver struct{}

func New() database.Driver {
	return &Driver{}
}

func (d *Driver) Dialect() database.Dialect {
	return database.DialectPostgreSQL
}

func (d *Driver) DSN(cfg *database.ConnectionConfig) string {
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	auth := cfg.User
	if cfg.Password != "" {
		auth += ":" + cfg.Password
	}
	dbName := cfg.Database
	if dbName == "" {
		dbName = "postgres"
	}
	return fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=%s", auth, host, port, dbName, sslMode)
}

func (d *Driver) Connect(ctx context.Context, cfg *database.ConnectionConfig) (*sql.DB, error) {
	dsn := d.DSN(cfg)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL: %w", err)
	}

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(tCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	if cfg.ReadOnly {
		_, _ = db.ExecContext(ctx, "SET default_transaction_read_only = TRUE;")
	}

	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

func (d *Driver) Version(ctx context.Context, db *sql.DB) (string, error) {
	var ver string
	err := db.QueryRowContext(ctx, "SELECT version()").Scan(&ver)
	if err != nil {
		return "", err
	}
	return ver, nil
}

func (d *Driver) Databases(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT datname 
		FROM pg_database 
		WHERE datistemplate = false 
		ORDER BY datname ASC;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		databases = append(databases, name)
	}
	return databases, nil
}

func (d *Driver) Tables(ctx context.Context, db *sql.DB) ([]database.TableInfo, error) {
	query := `
		SELECT 
			c.relname AS table_name,
			CASE c.relkind 
				WHEN 'r' THEN 'BASE TABLE' 
				WHEN 'v' THEN 'VIEW' 
				WHEN 'm' THEN 'MATERIALIZED VIEW' 
				ELSE 'OTHER' 
			END AS table_type,
			c.reltuples::bigint AS row_count,
			pg_relation_size(c.oid) AS data_size,
			pg_indexes_size(c.oid) AS index_size,
			COALESCE(obj_description(c.oid, 'pg_class'), '') AS comment
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' 
		  AND c.relkind IN ('r', 'v', 'm')
		ORDER BY c.relname ASC;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list postgres tables: %w", err)
	}
	defer rows.Close()

	var tables []database.TableInfo
	for rows.Next() {
		var t database.TableInfo
		if err := rows.Scan(&t.Name, &t.Type, &t.RowCount, &t.DataSize, &t.IndexSize, &t.Comment); err != nil {
			return nil, err
		}
		t.Engine = "PostgreSQL"
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func (d *Driver) DescribeTable(ctx context.Context, db *sql.DB, table string) (*database.TableDetail, error) {
	query := `
		SELECT 
			column_name, 
			ordinal_position, 
			data_type, 
			udt_name, 
			is_nullable, 
			column_default,
			character_maximum_length, 
			numeric_precision, 
			numeric_scale
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position ASC;
	`
	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table columns: %w", err)
	}
	defer rows.Close()

	var columns []database.ColumnInfo
	for rows.Next() {
		var name, dataType, udtName, isNullable string
		var ordPos int
		var dflt sql.NullString
		var charMax, numPrec, numScale sql.NullInt64

		if err := rows.Scan(&name, &ordPos, &dataType, &udtName, &isNullable, &dflt, &charMax, &numPrec, &numScale); err != nil {
			return nil, err
		}

		var defaultVal *string
		if dflt.Valid {
			v := dflt.String
			defaultVal = &v
		}

		c := database.ColumnInfo{
			Name:         name,
			Position:     ordPos,
			DataType:     strings.ToUpper(dataType),
			RawType:      udtName,
			IsNullable:   strings.ToUpper(isNullable) == "YES",
			DefaultValue: defaultVal,
		}
		if defaultVal != nil && strings.Contains(strings.ToLower(*defaultVal), "nextval(") {
			c.IsAutoIncr = true
		}
		if charMax.Valid {
			c.CharacterMax = &charMax.Int64
		}
		if numPrec.Valid {
			c.NumericPrec = &numPrec.Int64
		}
		if numScale.Valid {
			c.NumericScale = &numScale.Int64
		}
		columns = append(columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	indexes, err := d.Indexes(ctx, db, table)
	if err != nil {
		return nil, err
	}

	fks, err := d.ForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}

	var pkCols []string
	for _, idx := range indexes {
		if idx.IsPrimary {
			pkCols = idx.Columns
			for _, pkCol := range idx.Columns {
				for i := range columns {
					if columns[i].Name == pkCol {
						columns[i].IsPrimaryKey = true
					}
				}
			}
		}
		if idx.IsUnique && len(idx.Columns) == 1 {
			for i := range columns {
				if columns[i].Name == idx.Columns[0] {
					columns[i].IsUnique = true
				}
			}
		}
	}

	for _, fk := range fks {
		for _, col := range fk.Columns {
			for i := range columns {
				if columns[i].Name == col {
					columns[i].IsForeignKey = true
				}
			}
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
			Engine:     "PostgreSQL",
		},
		Columns:     columns,
		Indexes:     indexes,
		ForeignKeys: fks,
	}, nil
}

func (d *Driver) Indexes(ctx context.Context, db *sql.DB, table string) ([]database.IndexInfo, error) {
	query := `
		SELECT 
			i.relname AS index_name,
			ix.indisprimary AS is_primary,
			ix.indisunique AS is_unique,
			am.amname AS index_type,
			ARRAY_TO_STRING(ARRAY_AGG(a.attname ORDER BY array_position(ix.indkey, a.attnum)), ',') AS columns
		FROM pg_index ix
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_am am ON am.oid = i.relam
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		WHERE n.nspname = 'public' AND t.relname = $1
		GROUP BY i.relname, ix.indisprimary, ix.indisunique, am.amname
		ORDER BY i.relname ASC;
	`
	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to list postgres indexes: %w", err)
	}
	defer rows.Close()

	var indexes []database.IndexInfo
	for rows.Next() {
		var idxName, idxType, colsStr string
		var isPK, isUnique bool

		if err := rows.Scan(&idxName, &isPK, &isUnique, &idxType, &colsStr); err != nil {
			return nil, err
		}

		cols := strings.Split(colsStr, ",")
		indexes = append(indexes, database.IndexInfo{
			Name:      idxName,
			TableName: table,
			Columns:   cols,
			IsPrimary: isPK,
			IsUnique:  isUnique,
			Type:      strings.ToUpper(idxType),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return indexes, nil
}

func (d *Driver) ForeignKeys(ctx context.Context, db *sql.DB, table string) ([]database.ForeignKeyInfo, error) {
	query := `
		SELECT
			tc.constraint_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name,
			rc.update_rule,
			rc.delete_rule
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
		  ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
		  ON ccu.constraint_name = tc.constraint_name
		  AND ccu.table_schema = tc.table_schema
		JOIN information_schema.referential_constraints AS rc
		  ON rc.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema = 'public'
		  AND tc.table_name = $1
		ORDER BY tc.constraint_name, kcu.ordinal_position;
	`
	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query foreign keys: %w", err)
	}
	defer rows.Close()

	fkMap := make(map[string]*database.ForeignKeyInfo)
	var orderedNames []string

	for rows.Next() {
		var constraintName, colName, refTable, refCol, onUpdate, onDelete string
		if err := rows.Scan(&constraintName, &colName, &refTable, &refCol, &onUpdate, &onDelete); err != nil {
			return nil, err
		}

		fk, exists := fkMap[constraintName]
		if !exists {
			fk = &database.ForeignKeyInfo{
				Name:         constraintName,
				TableName:    table,
				RefTableName: refTable,
				OnUpdate:     onUpdate,
				OnDelete:     onDelete,
			}
			fkMap[constraintName] = fk
			orderedNames = append(orderedNames, constraintName)
		}
		fk.Columns = append(fk.Columns, colName)
		fk.RefColumns = append(fk.RefColumns, refCol)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var fks []database.ForeignKeyInfo
	for _, name := range orderedNames {
		fks = append(fks, *fkMap[name])
	}
	return fks, nil
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

	for _, t := range tables {
		fks, err := d.ForeignKeys(ctx, db, t.Name)
		if err == nil {
			for _, fk := range fks {
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
		}

		detail, err := d.DescribeTable(ctx, db, t.Name)
		if err == nil {
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
					}

					if targetTable != "" && !strings.EqualFold(targetTable, t.Name) {
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
	explainCmd := "EXPLAIN (FORMAT JSON"
	if analyze {
		explainCmd += ", ANALYZE, BUFFERS"
	}
	explainCmd += ") " + query

	var jsonOutput string
	err := db.QueryRowContext(ctx, explainCmd).Scan(&jsonOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to run PostgreSQL EXPLAIN: %w", err)
	}

	res := &database.ExplainResult{
		Dialect:   database.DialectPostgreSQL,
		RawOutput: jsonOutput,
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOutput), &parsed); err == nil && len(parsed) > 0 {
		plan, ok := parsed[0]["Plan"].(map[string]interface{})
		if ok {
			root := d.parsePostgresPlanNode(plan, res)
			res.RootNode = root
		}
	}

	return res, nil
}

func (d *Driver) parsePostgresPlanNode(nodeMap map[string]interface{}, res *database.ExplainResult) *database.PlanNode {
	node := &database.PlanNode{}
	if op, ok := nodeMap["Node Type"].(string); ok {
		node.Operation = op
		if op == "Seq Scan" {
			res.HasFullScan = true
			if rel, ok := nodeMap["Relation Name"].(string); ok {
				res.FullScanTabs = append(res.FullScanTabs, rel)
			}
		}
		if op == "Sort" {
			res.UsesFilesort = true
		}
	}
	if rel, ok := nodeMap["Relation Name"].(string); ok {
		node.Table = rel
	}
	if idx, ok := nodeMap["Index Name"].(string); ok {
		node.Index = idx
	}
	if cost, ok := nodeMap["Total Cost"].(float64); ok {
		node.Cost = cost
		if cost > res.TotalCost {
			res.TotalCost = cost
		}
	}
	if estRows, ok := nodeMap["Plan Rows"].(float64); ok {
		node.EstRows = estRows
	}
	if actRows, ok := nodeMap["Actual Rows"].(float64); ok {
		node.ActualRows = actRows
		res.RowsReturned = int64(actRows)
	}
	if actTime, ok := nodeMap["Actual Total Time"].(float64); ok {
		node.ActualTimeMs = actTime
		if actTime > res.TotalTimeMs {
			res.TotalTimeMs = actTime
		}
	}

	if plans, ok := nodeMap["Plans"].([]interface{}); ok {
		for _, child := range plans {
			if childMap, ok := child.(map[string]interface{}); ok {
				node.Children = append(node.Children, d.parsePostgresPlanNode(childMap, res))
			}
		}
	}
	return node
}

func (d *Driver) TableStats(ctx context.Context, db *sql.DB, table string) (*database.TableStats, error) {
	query := `
		SELECT 
			c.reltuples::bigint,
			pg_total_relation_size(c.oid),
			pg_relation_size(c.oid),
			pg_indexes_size(c.oid)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1;
	`
	var rows, totalSize, dataSize, idxSize int64
	err := db.QueryRowContext(ctx, query, table).Scan(&rows, &totalSize, &dataSize, &idxSize)
	if err != nil {
		return nil, err
	}

	return &database.TableStats{
		TableName:     table,
		EstimatedRows: rows,
		TotalBytes:    totalSize,
		DataBytes:     dataSize,
		IndexBytes:    idxSize,
	}, nil
}

var (
	pgEmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	pgUUIDRegex  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	pgDateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T|\s)\d{2}:\d{2}:\d{2}`)
)

func (d *Driver) SampleColumnData(ctx context.Context, db *sql.DB, table, col string, limit int) (*database.ColumnSampleStats, error) {
	if limit <= 0 {
		limit = 1000
	}

	query := fmt.Sprintf("SELECT \"%s\"::text FROM \"%s\" LIMIT %d", col, table, limit)
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

		strTrim := strings.TrimSpace(str)
		if stats.IsNumeric && !database.IsNumericString(strTrim) {
			stats.IsNumeric = false
		}
		if stats.IsBoolean && !database.IsBooleanString(strTrim) {
			stats.IsBoolean = false
		}
		if stats.IsDateTime && !pgDateRegex.MatchString(strTrim) {
			stats.IsDateTime = false
		}
		if stats.IsUUID && !pgUUIDRegex.MatchString(strTrim) {
			stats.IsUUID = false
		}
		if stats.IsEmail && !pgEmailRegex.MatchString(strTrim) {
			stats.IsEmail = false
		}
		if stats.IsJSON && !(strings.HasPrefix(strTrim, "{") && strings.HasSuffix(strTrim, "}")) &&
			!(strings.HasPrefix(strTrim, "[") && strings.HasSuffix(strTrim, "]")) {
			stats.IsJSON = false
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to sample column %s: %w", col, err)
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
