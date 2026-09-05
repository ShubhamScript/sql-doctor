package parser

import (
	"fmt"
	"strings"
)

// LintSeverity represents the impact of a lint violation
type LintSeverity string

const (
	SeverityCritical LintSeverity = "CRITICAL"
	SeverityWarning  LintSeverity = "WARNING"
	SeverityInfo     LintSeverity = "INFO"
)

// LintFinding describes a specific anti-pattern found in SQL
type LintFinding struct {
	RuleID      string       `json:"rule_id"`
	Severity    LintSeverity `json:"severity"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Suggestion  string       `json:"suggestion"`
	Line        int          `json:"line,omitempty"`
}

// Linter performs static AST and pattern analysis on SQL queries
type Linter struct {
	parser Parser
}

func NewLinter(p Parser) *Linter {
	if p == nil {
		p = NewVitessParser()
	}
	return &Linter{parser: p}
}

func (l *Linter) Lint(sqlText string) ([]LintFinding, error) {
	parsed, err := l.parser.Parse(sqlText)
	if err != nil {
		return []LintFinding{
			{
				RuleID:      "L000",
				Severity:    SeverityCritical,
				Title:       "Syntax Error",
				Description: fmt.Sprintf("SQL failed to parse cleanly: %v", err),
				Suggestion:  "Review query syntax against target database SQL specifications.",
			},
		}, nil
	}

	var findings []LintFinding

	// L001: SELECT * Anti-Pattern
	if parsed.HasSelectAll {
		findings = append(findings, LintFinding{
			RuleID:      "L001",
			Severity:    SeverityWarning,
			Title:       "SELECT * Wildcard Usage",
			Description: "Query uses 'SELECT *' instead of explicitly enumerating needed columns.",
			Suggestion:  "Explicitly specify required column names to reduce network transfer, memory pressure, and allow covering index scans.",
		})
	}

	// L002: Implicit Comma Joins
	if parsed.HasImplicitJn {
		findings = append(findings, LintFinding{
			RuleID:      "L002",
			Severity:    SeverityWarning,
			Title:       "Implicit Comma Join",
			Description: "Query joins tables using comma notation ('FROM a, b') instead of ANSI SQL-92 JOIN syntax.",
			Suggestion:  "Replace implicit comma joins with explicit 'INNER JOIN ... ON ...' to prevent unintended cartesian products and enhance clarity.",
		})
	}

	// L003: Cartesian Join
	if parsed.HasCrossJoin {
		findings = append(findings, LintFinding{
			RuleID:      "L003",
			Severity:    SeverityCritical,
			Title:       "Possible Cartesian Cross Join",
			Description: "Join expression is missing an 'ON' predicate, risking an unintentional cross product.",
			Suggestion:  "Ensure every JOIN has an explicit 'ON' condition matching foreign or primary keys.",
		})
	}

	// L004: Unqualified Column in Multi-table Queries
	if len(parsed.Tables) > 1 {
		unqualified := false
		for _, col := range parsed.Columns {
			if col.Table == "" && col.Column != "*" {
				unqualified = true
				break
			}
		}
		if unqualified {
			findings = append(findings, LintFinding{
				RuleID:      "L004",
				Severity:    SeverityInfo,
				Title:       "Unqualified Column References",
				Description: "Multiple tables are queried, but some columns are not prefixed with table names or aliases.",
				Suggestion:  "Qualify column references with table aliases (e.g. 'u.id' instead of 'id') to prevent schema ambiguity.",
			})
		}
	}

	// L005: Destructive UPDATE or DELETE without WHERE
	if (parsed.Type == StmtUpdate || parsed.Type == StmtDelete) && !parsed.HasWhere {
		findings = append(findings, LintFinding{
			RuleID:      "L005",
			Severity:    SeverityCritical,
			Title:       fmt.Sprintf("Dangerous %s Without WHERE Clause", parsed.Type),
			Description: fmt.Sprintf("%s statement lacks a WHERE clause and will affect ALL records in the target table!", parsed.Type),
			Suggestion:  "Add an explicit WHERE condition, or verify if a table-wide mutation was strictly intended.",
		})
	}

	// L006: Function-Wrapped Column in Predicate (SARGability issue)
	for _, pred := range parsed.Predicates {
		if pred.HasFunction {
			findings = append(findings, LintFinding{
				RuleID:      "L006",
				Severity:    SeverityWarning,
				Title:       "Non-SARGable Predicate Function Call",
				Description: fmt.Sprintf("Column '%s' is wrapped inside a function in the WHERE clause, which invalidates standard B-Tree index range scans.", pred.Column),
				Suggestion:  "Rewrite the predicate to isolate the column (e.g., use 'col >= ? AND col < ?' instead of 'YEAR(col) = ?').",
			})
			break
		}
	}

	// L007: Leading Wildcard in LIKE
	upperSQL := strings.ToUpper(sqlText)
	if strings.Contains(upperSQL, "LIKE '%") || strings.Contains(upperSQL, `LIKE "%`) {
		findings = append(findings, LintFinding{
			RuleID:      "L007",
			Severity:    SeverityWarning,
			Title:       "Leading Wildcard in LIKE Clause",
			Description: "Predicate contains LIKE '%...', which prevents index prefix scanning and forces a full table scan.",
			Suggestion:  "Consider using full-text indexing, trigram indexing, or trailing wildcards ('term%') where possible.",
		})
	}

	return findings, nil
}
