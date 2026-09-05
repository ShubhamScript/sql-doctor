package unit

import (
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/query/parser"
)

func TestVitessParser(t *testing.T) {
	p := parser.NewVitessParser()

	q := "SELECT id, name FROM users WHERE status = 'active' AND age > 21 ORDER BY created_at DESC;"
	parsed, err := p.Parse(q)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if parsed.Type != parser.StmtSelect {
		t.Errorf("expected StmtSelect, got %v", parsed.Type)
	}

	if len(parsed.Tables) != 1 || parsed.Tables[0] != "users" {
		t.Errorf("expected table 'users', got %v", parsed.Tables)
	}

	if len(parsed.Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(parsed.Predicates))
	}

	if parsed.Fingerprint == "" {
		t.Errorf("expected non-empty fingerprint")
	}
}

func TestParserSelectAll(t *testing.T) {
	p := parser.NewVitessParser()
	parsed, err := p.Parse("SELECT * FROM users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !parsed.HasSelectAll {
		t.Errorf("expected HasSelectAll = true")
	}
}
