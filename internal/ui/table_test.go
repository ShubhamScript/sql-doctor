package ui_test

import (
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/ui"
)

func TestTableRenderNormal(t *testing.T) {
	tbl := ui.NewTable("TABLE NAME", "TYPE", "EST. ROWS", "ENGINE")
	tbl.AddRow("users", "BASE TABLE", "10", "InnoDB")
	tbl.AddRow("orders", "BASE TABLE", "25", "InnoDB")

	out := tbl.Render()
	if out == "" {
		t.Fatalf("expected non-empty rendered table")
	}
}

func TestTableRenderWide(t *testing.T) {
	tbl := ui.NewTable("id", "name", "email", "email_verified_at", "password", "remember_token", "created_at", "updated_at")
	tbl.AddRow(
		"1", "Test User", "test@example.com", "2026-08-02 05:23:56",
		"$2y$12$3du298jbRmvwOjFIXSf4yesgjvorNji7pCLEgTe5l/yAtdv9xnCbq", "RCfBpIN8K2",
		"2026-08-02 05:23:56", "2026-08-02 05:23:56",
	)

	out := tbl.Render()
	if out == "" {
		t.Fatalf("expected non-empty rendered table")
	}
}
