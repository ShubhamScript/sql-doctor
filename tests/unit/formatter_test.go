package unit

import (
	"strings"
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/query/parser"
)

func TestFormatter(t *testing.T) {
	formatter := parser.NewFormatter()
	input := "select id, name from users where status='active' and age > 18 order by id desc"
	formatted := formatter.Format(input)

	if !strings.Contains(formatted, "SELECT id, name") {
		t.Errorf("expected uppercase SELECT, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "\nFROM users") {
		t.Errorf("expected line break for FROM, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "\nWHERE status='active'") {
		t.Errorf("expected line break for WHERE, got:\n%s", formatted)
	}
}
