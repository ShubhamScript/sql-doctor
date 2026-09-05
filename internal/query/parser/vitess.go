package parser

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"github.com/blastrain/vitess-sqlparser/sqlparser"
)

type VitessParser struct{}

func NewVitessParser() *VitessParser {
	return &VitessParser{}
}

func (p *VitessParser) Parse(query string) (*ParsedQuery, error) {
	cleanSQL := strings.TrimSpace(query)
	cleanSQL = strings.TrimSuffix(cleanSQL, ";")

	result := &ParsedQuery{
		RawSQL:      query,
		Type:        StmtUnknown,
		Fingerprint: p.Fingerprint(query),
	}

	stmt, err := sqlparser.Parse(cleanSQL)
	if err != nil {
		// Fallback to regex/heuristic parsing for statements Vitess parser doesn't support
		return p.fallbackParse(cleanSQL, result)
	}

	switch s := stmt.(type) {
	case *sqlparser.Select:
		result.Type = StmtSelect
		p.inspectSelect(s, result)
	case *sqlparser.Insert:
		result.Type = StmtInsert
		result.Tables = append(result.Tables, s.Table.Name.String())
	case *sqlparser.Update:
		result.Type = StmtUpdate
		for _, expr := range s.TableExprs {
			if aliased, ok := expr.(*sqlparser.AliasedTableExpr); ok {
				if tbl, ok := aliased.Expr.(sqlparser.TableName); ok {
					result.Tables = append(result.Tables, tbl.Name.String())
				}
			}
		}
		result.HasWhere = s.Where != nil
		if s.Where != nil {
			p.inspectWhere(s.Where.Expr, result)
		}
	case *sqlparser.Delete:
		result.Type = StmtDelete
		for _, expr := range s.TableExprs {
			if aliased, ok := expr.(*sqlparser.AliasedTableExpr); ok {
				if tbl, ok := aliased.Expr.(sqlparser.TableName); ok {
					result.Tables = append(result.Tables, tbl.Name.String())
				}
			}
		}
		result.HasWhere = s.Where != nil
		if s.Where != nil {
			p.inspectWhere(s.Where.Expr, result)
		}
	case *sqlparser.DDL:
		action := strings.ToUpper(s.Action)
		switch action {
		case "ALTER":
			result.Type = StmtAlter
		case "DROP":
			result.Type = StmtDrop
		case "CREATE":
			result.Type = StmtCreate
		default:
			result.Type = StatementType(action)
		}
		result.Tables = append(result.Tables, s.Table.Name.String())
	default:
		return p.fallbackParse(cleanSQL, result)
	}

	return result, nil
}

func (p *VitessParser) inspectSelect(s *sqlparser.Select, res *ParsedQuery) {
	// 1. Check Select Expressions (detect SELECT *)
	for _, expr := range s.SelectExprs {
		if _, ok := expr.(*sqlparser.StarExpr); ok {
			res.HasSelectAll = true
		} else if aliased, ok := expr.(*sqlparser.AliasedExpr); ok {
			if col, ok := aliased.Expr.(*sqlparser.ColName); ok {
				res.Columns = append(res.Columns, ColumnUsage{
					Table:   col.Qualifier.Name.String(),
					Column:  col.Name.String(),
					Context: "SELECT",
				})
			}
		}
	}

	// 2. Check FROM tables and Join conditions
	tableCount := len(s.From)
	if tableCount > 1 {
		res.HasImplicitJn = true
	}

	for _, tableExpr := range s.From {
		p.inspectTableExpr(tableExpr, res)
	}

	// 3. Check WHERE predicates
	if s.Where != nil {
		res.HasWhere = true
		p.inspectWhere(s.Where.Expr, res)
	}

	// 4. Check ORDER BY
	for _, order := range s.OrderBy {
		if col, ok := order.Expr.(*sqlparser.ColName); ok {
			res.OrderByCols = append(res.OrderByCols, col.Name.String())
			res.Columns = append(res.Columns, ColumnUsage{
				Table:   col.Qualifier.Name.String(),
				Column:  col.Name.String(),
				Context: "ORDER BY",
			})
		}
	}

	// 5. Check GROUP BY
	for _, group := range s.GroupBy {
		if col, ok := group.(*sqlparser.ColName); ok {
			res.GroupByCols = append(res.GroupByCols, col.Name.String())
			res.Columns = append(res.Columns, ColumnUsage{
				Table:   col.Qualifier.Name.String(),
				Column:  col.Name.String(),
				Context: "GROUP BY",
			})
		}
	}
}

func (p *VitessParser) inspectTableExpr(expr sqlparser.TableExpr, res *ParsedQuery) {
	switch t := expr.(type) {
	case *sqlparser.AliasedTableExpr:
		if tbl, ok := t.Expr.(sqlparser.TableName); ok {
			res.Tables = append(res.Tables, tbl.Name.String())
		}
	case *sqlparser.JoinTableExpr:
		p.inspectTableExpr(t.LeftExpr, res)
		p.inspectTableExpr(t.RightExpr, res)
		if t.Join == sqlparser.JoinStr || t.Join == sqlparser.StraightJoinStr {
			if t.On == nil {
				res.HasCrossJoin = true
			}
		}
	case *sqlparser.ParenTableExpr:
		for _, subExpr := range t.Exprs {
			p.inspectTableExpr(subExpr, res)
		}
	}
}

func (p *VitessParser) inspectWhere(expr sqlparser.Expr, res *ParsedQuery) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *sqlparser.AndExpr:
		p.inspectWhere(e.Left, res)
		p.inspectWhere(e.Right, res)
	case *sqlparser.OrExpr:
		p.inspectWhere(e.Left, res)
		p.inspectWhere(e.Right, res)
	case *sqlparser.ComparisonExpr:
		pred := Predicate{Operator: e.Operator}
		if col, ok := e.Left.(*sqlparser.ColName); ok {
			pred.Table = col.Qualifier.Name.String()
			pred.Column = col.Name.String()
			res.Columns = append(res.Columns, ColumnUsage{
				Table:    pred.Table,
				Column:   pred.Column,
				Context:  "WHERE",
				Operator: pred.Operator,
			})
		} else if _, ok := e.Left.(*sqlparser.FuncExpr); ok {
			pred.HasFunction = true
			// Check if column is wrapped in func
			_ = sqlparser.Walk(func(n sqlparser.SQLNode) (bool, error) {
				if c, ok := n.(*sqlparser.ColName); ok {
					pred.Table = c.Qualifier.Name.String()
					pred.Column = c.Name.String()
					return false, nil
				}
				return true, nil
			}, e.Left)
		}
		if pred.Column != "" {
			res.Predicates = append(res.Predicates, pred)
		}
	case *sqlparser.ParenExpr:
		p.inspectWhere(e.Expr, res)
	}
}

func (p *VitessParser) fallbackParse(sql string, res *ParsedQuery) (*ParsedQuery, error) {
	upper := strings.ToUpper(sql)
	fields := strings.Fields(upper)
	if len(fields) > 0 {
		switch fields[0] {
		case "SELECT":
			res.Type = StmtSelect
		case "INSERT":
			res.Type = StmtInsert
		case "UPDATE":
			res.Type = StmtUpdate
		case "DELETE":
			res.Type = StmtDelete
		case "ALTER":
			res.Type = StmtAlter
		case "DROP":
			res.Type = StmtDrop
		case "CREATE":
			res.Type = StmtCreate
		}
	}

	res.HasSelectAll = strings.Contains(upper, "SELECT *") || strings.Contains(upper, "SELECT  *")
	res.HasWhere = strings.Contains(upper, " WHERE ")

	// Heuristic table extraction
	fromRe := regexp.MustCompile(`(?i)\bFROM\s+([a-zA-Z0-9_\."]+)`)
	if matches := fromRe.FindStringSubmatch(sql); len(matches) > 1 {
		tbl := strings.Trim(matches[1], `"'` + "`")
		res.Tables = append(res.Tables, tbl)
	}

	return res, nil
}

var (
	numRegex = regexp.MustCompile(`\b\d+\b`)
	strRegex = regexp.MustCompile(`'[^']*'|"[^"]*"`)
)

func (p *VitessParser) Fingerprint(query string) string {
	clean := strings.TrimSpace(query)
	clean = strings.TrimSuffix(clean, ";")
	norm := strRegex.ReplaceAllString(clean, "?")
	norm = numRegex.ReplaceAllString(norm, "?")
	norm = strings.Join(strings.Fields(norm), " ")
	norm = strings.ToUpper(norm)

	h := sha256.Sum256([]byte(norm))
	return fmt.Sprintf("%x", h[:8])
}
