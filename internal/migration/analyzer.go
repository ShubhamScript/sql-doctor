package migration

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SafetyRiskLevel indicates the risk severity of a migration statement
type SafetyRiskLevel string

const (
	RiskCritical SafetyRiskLevel = "CRITICAL"
	RiskWarning  SafetyRiskLevel = "WARNING"
	RiskInfo     SafetyRiskLevel = "INFO"
)

// MigrationRisk describes an identified hazard in a migration script
type MigrationRisk struct {
	LineNumber  int             `json:"line_number"`
	Statement   string          `json:"statement"`
	RiskLevel   SafetyRiskLevel `json:"risk_level"`
	Hazard      string          `json:"hazard"`
	Description string          `json:"description"`
	Remedy      string          `json:"remedy"`
}

// MigrationReport summarizes migration safety findings
type MigrationReport struct {
	FilePath    string          `json:"file_path"`
	TotalRisks  int             `json:"total_risks"`
	IsSafe      bool            `json:"is_safe"`
	Risks       []MigrationRisk `json:"risks"`
}

// Analyzer inspects migration files for locking and destructive risks
type Analyzer struct{}

func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

func (a *Analyzer) AnalyzeFile(path string) (*MigrationReport, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open migration file: %w", err)
	}
	defer file.Close()

	report := &MigrationReport{
		FilePath: path,
		IsSafe:   true,
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	var currentStmt strings.Builder
	stmtStartLine := 1

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if strings.HasPrefix(line, "--") || strings.HasPrefix(line, "/*") || line == "" {
			continue
		}

		if currentStmt.Len() == 0 {
			stmtStartLine = lineNum
		}
		currentStmt.WriteByte(' ')
		currentStmt.WriteString(line)

		if strings.HasSuffix(line, ";") {
			stmt := strings.TrimSpace(currentStmt.String())
			a.checkStatement(stmt, stmtStartLine, report)
			currentStmt.Reset()
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read migration file: %w", err)
	}

	if currentStmt.Len() > 0 {
		stmt := strings.TrimSpace(currentStmt.String())
		a.checkStatement(stmt, stmtStartLine, report)
	}

	report.TotalRisks = len(report.Risks)
	for _, r := range report.Risks {
		if r.RiskLevel == RiskCritical {
			report.IsSafe = false
			break
		}
	}

	return report, nil
}

func (a *Analyzer) checkStatement(stmt string, line int, report *MigrationReport) {
	upper := strings.ToUpper(stmt)

	// 1. DROP TABLE
	if strings.Contains(upper, "DROP TABLE") {
		report.Risks = append(report.Risks, MigrationRisk{
			LineNumber:  line,
			Statement:   truncate(stmt, 80),
			RiskLevel:   RiskCritical,
			Hazard:      "Destructive DROP TABLE",
			Description: "Permanently destroys a table and all its stored records and indexes.",
			Remedy:      "Ensure backups exist and verify this table is completely deprecated before dropping.",
		})
	}

	// 2. DROP COLUMN
	if strings.Contains(upper, "DROP COLUMN") || regexp.MustCompile(`ALTER\s+TABLE\s+\S+\s+DROP\s+\S+`).MatchString(upper) {
		report.Risks = append(report.Risks, MigrationRisk{
			LineNumber:  line,
			Statement:   truncate(stmt, 80),
			RiskLevel:   RiskCritical,
			Hazard:      "Destructive DROP COLUMN",
			Description: "Irreversibly deletes column data. Any running application code querying this column will immediately crash.",
			Remedy:      "Perform a multi-phase migration: 1. Deploy code that ignores the column. 2. Verify traffic. 3. Drop column.",
		})
	}

	// 3. TRUNCATE
	if strings.Contains(upper, "TRUNCATE ") || strings.Contains(upper, "TRUNCATE TABLE") {
		report.Risks = append(report.Risks, MigrationRisk{
			LineNumber:  line,
			Statement:   truncate(stmt, 80),
			RiskLevel:   RiskCritical,
			Hazard:      "Destructive TRUNCATE",
			Description: "Immediately empties all rows in the table without firing individual row-level delete triggers.",
			Remedy:      "Ensure TRUNCATE is only intended for transient/cache tables.",
		})
	}

	// 4. ALTER TABLE ADD COLUMN NOT NULL without DEFAULT
	if strings.Contains(upper, "ADD COLUMN") && strings.Contains(upper, "NOT NULL") && !strings.Contains(upper, "DEFAULT") {
		report.Risks = append(report.Risks, MigrationRisk{
			LineNumber:  line,
			Statement:   truncate(stmt, 80),
			RiskLevel:   RiskCritical,
			Hazard:      "Non-nullable Column Added Without Default",
			Description: "Adding a NOT NULL column without a DEFAULT will fail on tables containing existing rows, or cause an exclusive lock rewrite.",
			Remedy:      "Provide an explicit DEFAULT value, or add column as nullable first, backfill data, and then set NOT NULL.",
		})
	}

	// 5. Incompatible Type Conversions
	if strings.Contains(upper, "ALTER") && (strings.Contains(upper, "MODIFY") || strings.Contains(upper, "ALTER COLUMN")) {
		report.Risks = append(report.Risks, MigrationRisk{
			LineNumber:  line,
			Statement:   truncate(stmt, 80),
			RiskLevel:   RiskWarning,
			Hazard:      "Potential Table Rewrite & Lock",
			Description: "Modifying column data types or decreasing lengths can cause full table rewrites and exclusive metadata locks.",
			Remedy:      "Use online DDL tools (e.g. pt-online-schema-change or gh-ost) or perform in low-traffic maintenance windows.",
		})
	}

	// 6. Adding Foreign Key without NOT VALID (in Postgres)
	if strings.Contains(upper, "ADD CONSTRAINT") && strings.Contains(upper, "FOREIGN KEY") {
		report.Risks = append(report.Risks, MigrationRisk{
			LineNumber:  line,
			Statement:   truncate(stmt, 80),
			RiskLevel:   RiskWarning,
			Hazard:      "Foreign Key Validation Lock",
			Description: "Validating foreign keys locks tables against writes while inspecting all existing records.",
			Remedy:      "In PostgreSQL, add foreign keys with 'NOT VALID' first, then run 'VALIDATE CONSTRAINT' concurrently.",
		})
	}
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
