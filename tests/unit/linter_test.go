package unit

import (
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/query/parser"
)

func TestLinterRules(t *testing.T) {
	linter := parser.NewLinter(nil)

	// L001: SELECT *
	findings, _ := linter.Lint("SELECT * FROM users;")
	hasL001 := false
	for _, f := range findings {
		if f.RuleID == "L001" {
			hasL001 = true
		}
	}
	if !hasL001 {
		t.Errorf("expected L001 finding for SELECT *")
	}

	// L005: Destructive DELETE without WHERE
	findings, _ = linter.Lint("DELETE FROM users;")
	hasL005 := false
	for _, f := range findings {
		if f.RuleID == "L005" {
			hasL005 = true
		}
	}
	if !hasL005 {
		t.Errorf("expected L005 finding for DELETE without WHERE")
	}

	// L007: Leading Wildcard
	findings, _ = linter.Lint("SELECT id FROM users WHERE email LIKE '%example.com';")
	hasL007 := false
	for _, f := range findings {
		if f.RuleID == "L007" {
			hasL007 = true
		}
	}
	if !hasL007 {
		t.Errorf("expected L007 finding for leading wildcard")
	}
}
