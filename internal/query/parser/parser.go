package parser

// StatementType identifies SQL statement classes
type StatementType string

const (
	StmtSelect  StatementType = "SELECT"
	StmtInsert  StatementType = "INSERT"
	StmtUpdate  StatementType = "UPDATE"
	StmtDelete  StatementType = "DELETE"
	StmtAlter   StatementType = "ALTER"
	StmtDrop    StatementType = "DROP"
	StmtCreate  StatementType = "CREATE"
	StmtUnknown StatementType = "UNKNOWN"
)

// ColumnUsage denotes how a column is accessed in a query
type ColumnUsage struct {
	Table    string `json:"table,omitempty"`
	Column   string `json:"column"`
	Context  string `json:"context"` // "SELECT", "WHERE", "JOIN", "ORDER BY", "GROUP BY"
	Operator string `json:"operator,omitempty"` // "=", ">", "LIKE", "IN", etc.
}

// Predicate holds details about a filter condition
type Predicate struct {
	Table       string `json:"table,omitempty"`
	Column      string `json:"column"`
	Operator    string `json:"operator"`
	HasFunction bool   `json:"has_function"` // true if column wrapped in function e.g. YEAR(date_col)
}

// ParsedQuery contains structural AST insights
type ParsedQuery struct {
	RawSQL        string        `json:"raw_sql"`
	Type          StatementType `json:"type"`
	Tables        []string      `json:"tables"`
	Columns       []ColumnUsage `json:"columns"`
	Predicates    []Predicate   `json:"predicates"`
	OrderByCols   []string      `json:"order_by_columns,omitempty"`
	GroupByCols   []string      `json:"group_by_columns,omitempty"`
	HasSelectAll  bool          `json:"has_select_all"`
	HasImplicitJn bool          `json:"has_implicit_join"`
	HasCrossJoin  bool          `json:"has_cross_join"`
	HasWhere      bool          `json:"has_where"`
	Fingerprint   string        `json:"fingerprint"`
}

// Parser defines the SQL parsing interface
type Parser interface {
	Parse(sql string) (*ParsedQuery, error)
	Fingerprint(sql string) string
}
