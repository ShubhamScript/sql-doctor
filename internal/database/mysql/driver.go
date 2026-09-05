package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sql-doctor/sql-doctor/internal/database"
)

type Driver struct {
	dialect database.Dialect
}

func NewMySQL() database.Driver {
	return &Driver{dialect: database.DialectMySQL}
}

func NewMariaDB() database.Driver {
	return &Driver{dialect: database.DialectMariaDB}
}

func (d *Driver) Dialect() database.Dialect {
	return d.dialect
}

func (d *Driver) DSN(cfg *database.ConnectionConfig) string {
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port == 0 {
		port = 3306
	}
	auth := cfg.User
	if cfg.Password != "" {
		auth += ":" + cfg.Password
	}
	dsn := fmt.Sprintf("%s@tcp(%s:%d)/%s?parseTime=true", auth, host, port, cfg.Database)
	if cfg.SSLMode != "" && cfg.SSLMode != "disable" {
		dsn += "&tls=" + cfg.SSLMode
	}
	return dsn
}

func (d *Driver) Connect(ctx context.Context, cfg *database.ConnectionConfig) (*sql.DB, error) {
	dsn := d.DSN(cfg)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open MySQL/MariaDB: %w", err)
	}

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(tCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to MySQL/MariaDB (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	if cfg.ReadOnly {
		_, _ = db.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	}

	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

func (d *Driver) Version(ctx context.Context, db *sql.DB) (string, error) {
	var ver string
	var verComment sql.NullString
	_ = db.QueryRowContext(ctx, "SELECT @@version, @@version_comment").Scan(&ver, &verComment)
	if ver == "" {
		err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&ver)
		if err != nil {
			return "", err
		}
	}
	res := "MySQL " + ver
	if verComment.Valid && strings.Contains(strings.ToLower(verComment.String), "mariadb") {
		res = "MariaDB " + ver
	}
	return res, nil
}

func (d *Driver) Tables(ctx context.Context, db *sql.DB) ([]database.TableInfo, error) {
	query := `
		SELECT 
			TABLE_NAME, 
			TABLE_TYPE, 
			COALESCE(TABLE_ROWS, 0), 
			COALESCE(DATA_LENGTH, 0), 
			COALESCE(INDEX_LENGTH, 0), 
			COALESCE(ENGINE, ''), 
			COALESCE(TABLE_COLLATION, ''), 
			COALESCE(TABLE_COMMENT, '')
		FROM INFORMATION_SCHEMA.TABLES 
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME ASC;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tables []database.TableInfo
	for rows.Next() {
		var t database.TableInfo
		if err := rows.Scan(&t.Name, &t.Type, &t.RowCount, &t.DataSize, &t.IndexSize, &t.Engine, &t.Collation, &t.Comment); err != nil {
			return nil, err
		}
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
			COLUMN_NAME, 
			ORDINAL_POSITION, 
			DATA_TYPE, 
			COLUMN_TYPE, 
			IS_NULLABLE, 
			COLUMN_DEFAULT, 
			COLUMN_KEY, 
			EXTRA, 
			CHARACTER_MAXIMUM_LENGTH, 
			NUMERIC_PRECISION, 
			NUMERIC_SCALE, 
			COLUMN_COMMENT
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION ASC;
	`
	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table columns: %w", err)
	}
	defer rows.Close()

	var columns []database.ColumnInfo
	var pkCols []string

	for rows.Next() {
		var colName, isNullable, colKey, extra, dataType, colType, comment string
		var ordPos int
		var dflt sql.NullString
		var charMax, numPrec, numScale sql.NullInt64

		if err := rows.Scan(&colName, &ordPos, &dataType, &colType, &isNullable, &dflt, &colKey, &extra, &charMax, &numPrec, &numScale, &comment); err != nil {
			return nil, err
		}

		isPK := strings.ToUpper(colKey) == "PRI"
		if isPK {
			pkCols = append(pkCols, colName)
		}

		var defaultVal *string
		if dflt.Valid {
			v := dflt.String
			defaultVal = &v
		}

		c := database.ColumnInfo{
			Name:         colName,
			Position:     ordPos,
			DataType:     strings.ToUpper(dataType),
			RawType:      colType,
			IsNullable:   strings.ToUpper(isNullable) == "YES",
			DefaultValue: defaultVal,
			IsPrimaryKey: isPK,
			IsUnique:     strings.ToUpper(colKey) == "UNI",
			IsAutoIncr:   strings.Contains(strings.ToLower(extra), "auto_increment"),
			Comment:      comment,
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

	fkCols := make(map[string]bool)
	for _, fk := range fks {
		for _, col := range fk.Columns {
			fkCols[col] = true
		}
	}
	for i := range columns {
		if fkCols[columns[i].Name] {
			columns[i].IsForeignKey = true
		}
	}

	// Fetch table row count
	var rowCount int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(1) FROM `%s`", table)).Scan(&rowCount)

	return &database.TableDetail{
		Table: database.TableInfo{
			Name:       table,
			Type:       "BASE TABLE",
			RowCount:   rowCount,
			PrimaryKey: strings.Join(pkCols, ", "),
			Engine:     "InnoDB",
		},
		Columns:     columns,
		Indexes:     indexes,
		ForeignKeys: fks,
	}, nil
}

func (d *Driver) Indexes(ctx context.Context, db *sql.DB, table string) ([]database.IndexInfo, error) {
	query := `
		SELECT 
			INDEX_NAME, 
			NON_UNIQUE, 
			INDEX_TYPE, 
			COLUMN_NAME, 
			COALESCE(CARDINALITY, 0), 
			COALESCE(INDEX_COMMENT, '')
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY INDEX_NAME ASC, SEQ_IN_INDEX ASC;
	`
	rows, err := db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer rows.Close()

	idxMap := make(map[string]*database.IndexInfo)
	var orderedNames []string

	for rows.Next() {
		var idxName, idxType, colName, comment string
		var nonUnique int
		var card int64

		if err := rows.Scan(&idxName, &nonUnique, &idxType, &colName, &card, &comment); err != nil {
			return nil, err
		}

		idx, exists := idxMap[idxName]
		if !exists {
			idx = &database.IndexInfo{
				Name:        idxName,
				TableName:   table,
				IsPrimary:   idxName == "PRIMARY",
				IsUnique:    nonUnique == 0,
				Type:        idxType,
				Cardinality: card,
				Comment:     comment,
			}
			idxMap[idxName] = idx
			orderedNames = append(orderedNames, idxName)
		}
		idx.Columns = append(idx.Columns, colName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var indexes []database.IndexInfo
	for _, name := range orderedNames {
		indexes = append(indexes, *idxMap[name])
	}
	return indexes, nil
}

func (d *Driver) ForeignKeys(ctx context.Context, db *sql.DB, table string) ([]database.ForeignKeyInfo, error) {
	query := `
		SELECT 
			k.CONSTRAINT_NAME, 
			k.COLUMN_NAME, 
			k.REFERENCED_TABLE_NAME, 
			k.REFERENCED_COLUMN_NAME, 
			r.UPDATE_RULE, 
			r.DELETE_RULE
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE k
		JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS r 
		  ON k.CONSTRAINT_NAME = r.CONSTRAINT_NAME AND k.CONSTRAINT_SCHEMA = r.CONSTRAINT_SCHEMA
		WHERE k.TABLE_SCHEMA = DATABASE() AND k.TABLE_NAME = ? AND k.REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY k.CONSTRAINT_NAME ASC, k.ORDINAL_POSITION ASC;
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
						FROM %s AS a 
						LEFT JOIN %s AS b ON a.%s = b.%s 
						WHERE a.%s IS NOT NULL AND b.%s IS NULL
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

		// Inferred relationships
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
							FROM %s AS a 
							LEFT JOIN %s AS b ON a.%s = b.id 
							WHERE a.%s IS NOT NULL AND b.id IS NULL
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
	explainCmd := "EXPLAIN FORMAT=JSON " + query
	if analyze {
		// EXPLAIN ANALYZE is supported in MySQL 8.0.18+
		analyzeCmd := "EXPLAIN ANALYZE " + query
		var analyzeOutput string
		err := db.QueryRowContext(ctx, analyzeCmd).Scan(&analyzeOutput)
		if err == nil && analyzeOutput != "" {
			return &database.ExplainResult{
				Dialect:      database.DialectMySQL,
				RawOutput:    analyzeOutput,
				HasFullScan:  strings.Contains(analyzeOutput, "Table scan"),
				UsesFilesort: strings.Contains(analyzeOutput, "filesort"),
				UsesTemp:     strings.Contains(analyzeOutput, "temporary"),
			}, nil
		}
	}

	var jsonOutput string
	err := db.QueryRowContext(ctx, explainCmd).Scan(&jsonOutput)
	if err != nil {
		// Fallback to traditional tabular EXPLAIN
		return d.explainTabular(ctx, db, query)
	}

	res := &database.ExplainResult{
		Dialect:   database.DialectMySQL,
		RawOutput: jsonOutput,
	}

	// Parse MySQL EXPLAIN JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOutput), &parsed); err == nil {
		d.parseMySQLJSONPlan(parsed, res)
	}

	return res, nil
}

func (d *Driver) parseMySQLJSONPlan(data map[string]interface{}, res *database.ExplainResult) {
	queryBlock, ok := data["query_block"].(map[string]interface{})
	if !ok {
		return
	}

	if costStr, ok := queryBlock["cost_info"].(map[string]interface{}); ok {
		if qCost, ok := costStr["query_cost"].(string); ok {
			res.TotalCost, _ = strconv.ParseFloat(qCost, 64)
		}
	}

	rootNode := &database.PlanNode{Operation: "Query Block"}
	d.extractMySQLNodes(queryBlock, rootNode, res)
	res.RootNode = rootNode
}

func (d *Driver) extractMySQLNodes(block map[string]interface{}, parent *database.PlanNode, res *database.ExplainResult) {
	// Check table
	if tbl, ok := block["table"].(map[string]interface{}); ok {
		node := &database.PlanNode{}
		if name, ok := tbl["table_name"].(string); ok {
			node.Table = name
		}
		if accessType, ok := tbl["access_type"].(string); ok {
			node.ScanType = accessType
			if strings.EqualFold(accessType, "ALL") {
				res.HasFullScan = true
				res.FullScanTabs = append(res.FullScanTabs, node.Table)
			}
		}
		if key, ok := tbl["key"].(string); ok {
			node.Index = key
		}
		if rowsExam, ok := tbl["rows_examined_per_scan"].(float64); ok {
			node.EstRows = rowsExam
			res.RowsExamined += int64(rowsExam)
		}
		if tbl["using_filesort"] == true {
			res.UsesFilesort = true
		}
		if tbl["using_temporary_table"] == true {
			res.UsesTemp = true
		}
		parent.Children = append(parent.Children, node)
	}

	// Check nested loops
	if nested, ok := block["nested_loop"].([]interface{}); ok {
		for _, item := range nested {
			if itemMap, ok := item.(map[string]interface{}); ok {
				d.extractMySQLNodes(itemMap, parent, res)
			}
		}
	}
}

func (d *Driver) explainTabular(ctx context.Context, db *sql.DB, query string) (*database.ExplainResult, error) {
	rows, err := db.QueryContext(ctx, "EXPLAIN "+query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var rawLines []string
	rawLines = append(rawLines, strings.Join(cols, " | "))

	res := &database.ExplainResult{Dialect: database.DialectMySQL}
	rootNode := &database.PlanNode{Operation: "EXPLAIN"}

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		rowStr := make([]string, len(cols))
		var tblName, scanType, keyName, extra string
		var rowsExamined int64

		for i, v := range vals {
			strVal := ""
			if v != nil {
				strVal = fmt.Sprintf("%v", v)
			}
			rowStr[i] = strVal

			colUpper := strings.ToUpper(cols[i])
			switch colUpper {
			case "TABLE":
				tblName = strVal
			case "TYPE":
				scanType = strVal
				if strings.EqualFold(strVal, "ALL") {
					res.HasFullScan = true
					res.FullScanTabs = append(res.FullScanTabs, tblName)
				}
			case "KEY":
				keyName = strVal
			case "ROWS":
				rowsExamined, _ = strconv.ParseInt(strVal, 10, 64)
				res.RowsExamined += rowsExamined
			case "EXTRA":
				extra = strVal
				if strings.Contains(strings.ToLower(extra), "using filesort") {
					res.UsesFilesort = true
				}
				if strings.Contains(strings.ToLower(extra), "using temporary") {
					res.UsesTemp = true
				}
			}
		}

		rawLines = append(rawLines, strings.Join(rowStr, " | "))
		rootNode.Children = append(rootNode.Children, &database.PlanNode{
			Table:    tblName,
			ScanType: scanType,
			Index:    keyName,
			EstRows:  float64(rowsExamined),
			Extra:    extra,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res.RawOutput = strings.Join(rawLines, "\n")
	res.RootNode = rootNode
	return res, nil
}

func (d *Driver) TableStats(ctx context.Context, db *sql.DB, table string) (*database.TableStats, error) {
	query := `
		SELECT 
			COALESCE(TABLE_ROWS, 0), 
			COALESCE(DATA_LENGTH, 0), 
			COALESCE(INDEX_LENGTH, 0)
		FROM INFORMATION_SCHEMA.TABLES 
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?;
	`
	var rows, dataLen, idxLen int64
	err := db.QueryRowContext(ctx, query, table).Scan(&rows, &dataLen, &idxLen)
	if err != nil {
		return nil, err
	}

	return &database.TableStats{
		TableName:     table,
		EstimatedRows: rows,
		TotalBytes:    dataLen + idxLen,
		DataBytes:     dataLen,
		IndexBytes:    idxLen,
	}, nil
}

var (
	mysqlEmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	mysqlUUIDRegex  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	mysqlDateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T|\s)\d{2}:\d{2}:\d{2}`)
)

func (d *Driver) SampleColumnData(ctx context.Context, db *sql.DB, table, col string, limit int) (*database.ColumnSampleStats, error) {
	if limit <= 0 {
		limit = 1000
	}

	query := fmt.Sprintf("SELECT `%s` FROM `%s` LIMIT %d", col, table, limit)
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
		if stats.IsNumeric && !isNumericString(strTrim) {
			stats.IsNumeric = false
		}
		if stats.IsBoolean && !isBooleanString(strTrim) {
			stats.IsBoolean = false
		}
		if stats.IsDateTime && !mysqlDateRegex.MatchString(strTrim) {
			stats.IsDateTime = false
		}
		if stats.IsUUID && !mysqlUUIDRegex.MatchString(strTrim) {
			stats.IsUUID = false
		}
		if stats.IsEmail && !mysqlEmailRegex.MatchString(strTrim) {
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
