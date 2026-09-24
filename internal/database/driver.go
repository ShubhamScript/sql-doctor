package database

import (
	"context"
	"database/sql"
	"time"
)

// Dialect represents supported database engines
type Dialect string

const (
	DialectMySQL      Dialect = "mysql"
	DialectMariaDB    Dialect = "mariadb"
	DialectPostgreSQL Dialect = "postgres"
	DialectSQLite     Dialect = "sqlite"
)

// ConnectionConfig defines parameters for establishing a database connection
type ConnectionConfig struct {
	Name     string        `json:"name"`
	Dialect  Dialect       `json:"dialect"`
	Host     string        `json:"host"`
	Port     int           `json:"port"`
	User     string        `json:"user"`
	Password string        `json:"password"`
	Database string        `json:"database"`
	SSLMode  string        `json:"ssl_mode,omitempty"`
	FilePath string        `json:"file_path,omitempty"` // For SQLite
	ReadOnly bool          `json:"read_only"`
	Timeout  time.Duration `json:"timeout"`
}

// TableInfo provides high-level table metadata
type TableInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // BASE TABLE, VIEW, etc.
	RowCount   int64  `json:"row_count"`
	DataSize   int64  `json:"data_size_bytes"`
	IndexSize  int64  `json:"index_size_bytes"`
	Engine     string `json:"engine,omitempty"`
	Collation  string `json:"collation,omitempty"`
	Comment    string `json:"comment,omitempty"`
	PrimaryKey string `json:"primary_key,omitempty"`
}

// ColumnInfo holds details about a single table column
type ColumnInfo struct {
	Name          string  `json:"name"`
	Position      int     `json:"position"`
	DataType      string  `json:"data_type"`
	RawType       string  `json:"raw_type"` // e.g., varchar(255)
	IsNullable    bool    `json:"is_nullable"`
	DefaultValue  *string `json:"default_value"`
	IsPrimaryKey  bool    `json:"is_primary_key"`
	IsForeignKey  bool    `json:"is_foreign_key"`
	IsUnique      bool    `json:"is_unique"`
	IsAutoIncr    bool    `json:"is_auto_increment"`
	CharacterMax  *int64  `json:"char_max_length,omitempty"`
	NumericPrec   *int64  `json:"numeric_precision,omitempty"`
	NumericScale  *int64  `json:"numeric_scale,omitempty"`
	Comment       string  `json:"comment,omitempty"`
}

// IndexInfo holds details about an index
type IndexInfo struct {
	Name        string   `json:"name"`
	TableName   string   `json:"table_name"`
	Columns     []string `json:"columns"`
	IsPrimary   bool     `json:"is_primary"`
	IsUnique    bool     `json:"is_unique"`
	Type        string   `json:"type"` // BTREE, HASH, GIN, etc.
	Cardinality int64    `json:"cardinality"`
	Comment     string   `json:"comment,omitempty"`
}

// ForeignKeyInfo holds details about a foreign key constraint
type ForeignKeyInfo struct {
	Name             string   `json:"name"`
	TableName        string   `json:"table_name"`
	Columns          []string `json:"columns"`
	RefTableName     string   `json:"ref_table_name"`
	RefColumns       []string `json:"ref_columns"`
	OnUpdate         string   `json:"on_update"`
	OnDelete         string   `json:"on_delete"`
}

// RelationshipInfo encapsulates a detected relationship between two tables
type RelationshipInfo struct {
	FromTable   string   `json:"from_table"`
	FromColumns []string `json:"from_columns"`
	ToTable     string   `json:"to_table"`
	ToColumns   []string `json:"to_columns"`
	IsExplicit  bool     `json:"is_explicit"` // true = enforced via foreign key, false = inferred by naming convention
	Constraint  string   `json:"constraint_name,omitempty"`
	OrphanCount int64    `json:"orphan_count,omitempty"`
}

// TableDetail contains full schema metadata for a table
type TableDetail struct {
	Table       TableInfo        `json:"table"`
	Columns     []ColumnInfo     `json:"columns"`
	Indexes     []IndexInfo      `json:"indexes"`
	ForeignKeys []ForeignKeyInfo `json:"foreign_keys"`
}

// PlanNode represents a node in an execution plan tree
type PlanNode struct {
	Operation    string      `json:"operation"`
	Table        string      `json:"table,omitempty"`
	Index        string      `json:"index,omitempty"`
	Cost         float64     `json:"cost,omitempty"`
	ActualRows   float64     `json:"actual_rows,omitempty"`
	EstRows      float64     `json:"estimated_rows,omitempty"`
	ActualTimeMs float64     `json:"actual_time_ms,omitempty"`
	ScanType     string      `json:"scan_type,omitempty"` // ALL, Index, Seq Scan, etc.
	Filter       string      `json:"filter,omitempty"`
	KeyCols      []string    `json:"key_columns,omitempty"`
	Extra        string      `json:"extra,omitempty"`
	Children     []*PlanNode `json:"children,omitempty"`
}

// ExplainResult stores the output of an EXPLAIN query
type ExplainResult struct {
	Dialect      Dialect   `json:"dialect"`
	RawOutput    string    `json:"raw_output"`
	RootNode     *PlanNode `json:"root_node,omitempty"`
	TotalCost    float64   `json:"total_cost,omitempty"`
	TotalTimeMs  float64   `json:"total_time_ms,omitempty"`
	RowsExamined int64     `json:"rows_examined,omitempty"`
	RowsReturned int64     `json:"rows_returned,omitempty"`
	HasFullScan  bool      `json:"has_full_scan"`
	FullScanTabs []string  `json:"full_scan_tables,omitempty"`
	UsesTemp     bool      `json:"uses_temporary_table"`
	UsesFilesort bool      `json:"uses_filesort"`
}

// ColumnSampleStats holds statistical samples of actual data in a column
type ColumnSampleStats struct {
	ColumnName     string             `json:"column_name"`
	TotalSampled   int64              `json:"total_sampled"`
	NullCount      int64              `json:"null_count"`
	NullPercentage float64            `json:"null_percentage"`
	DistinctCount  int64              `json:"distinct_count"`
	MinLength      int                `json:"min_length"`
	MaxLength      int                `json:"max_length"`
	AvgLength      float64            `json:"avg_length"`
	MinValue       string             `json:"min_value,omitempty"`
	MaxValue       string             `json:"max_value,omitempty"`
	IsNumeric      bool               `json:"is_numeric"`
	IsBoolean      bool               `json:"is_boolean"`
	IsDateTime     bool               `json:"is_datetime"`
	IsUUID         bool               `json:"is_uuid"`
	IsEmail        bool               `json:"is_email"`
	IsJSON         bool               `json:"is_json"`
	SampleValues   []string           `json:"sample_values,omitempty"`
}

// TableStats stores statistical summary for a table
type TableStats struct {
	TableName    string `json:"table_name"`
	EstimatedRows int64 `json:"estimated_rows"`
	TotalBytes   int64  `json:"total_bytes"`
	DataBytes    int64  `json:"data_bytes"`
	IndexBytes   int64  `json:"index_bytes"`
}

// Driver provides the standard interface for database interaction
type Driver interface {
	Dialect() Dialect
	DSN(config *ConnectionConfig) string
	Connect(ctx context.Context, config *ConnectionConfig) (*sql.DB, error)
	Ping(ctx context.Context, db *sql.DB) error
	Version(ctx context.Context, db *sql.DB) (string, error)
	Databases(ctx context.Context, db *sql.DB) ([]string, error)
	Tables(ctx context.Context, db *sql.DB) ([]TableInfo, error)
	DescribeTable(ctx context.Context, db *sql.DB, table string) (*TableDetail, error)
	Indexes(ctx context.Context, db *sql.DB, table string) ([]IndexInfo, error)
	ForeignKeys(ctx context.Context, db *sql.DB, table string) ([]ForeignKeyInfo, error)
	Relationships(ctx context.Context, db *sql.DB) ([]RelationshipInfo, error)
	Explain(ctx context.Context, db *sql.DB, query string, analyze bool) (*ExplainResult, error)
	TableStats(ctx context.Context, db *sql.DB, table string) (*TableStats, error)
	SampleColumnData(ctx context.Context, db *sql.DB, table, col string, limit int) (*ColumnSampleStats, error)
}
