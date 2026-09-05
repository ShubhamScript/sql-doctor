package schema

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
)

// DatatypeRecommendation specifies an optimal datatype recommendation
type DatatypeRecommendation struct {
	TableName      string  `json:"table_name"`
	ColumnName     string  `json:"column_name"`
	CurrentType    string  `json:"current_type"`
	SuggestedType  string  `json:"suggested_type"`
	Confidence     int     `json:"confidence_percentage"` // 0-100
	ObservedStats  string  `json:"observed_stats"`
	Rationale      string  `json:"rationale"`
	FutureWarning  string  `json:"future_growth_warning"`
}

// Advisor analyzes actual column contents to suggest optimized schema types
type Advisor struct {
	driver database.Driver
}

func NewAdvisor(driver database.Driver) *Advisor {
	return &Advisor{driver: driver}
}

func (a *Advisor) AnalyzeTable(ctx context.Context, db *sql.DB, tableName string) ([]DatatypeRecommendation, error) {
	detail, err := a.driver.DescribeTable(ctx, db, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table '%s': %w", tableName, err)
	}

	var recommendations []DatatypeRecommendation

	for _, col := range detail.Columns {
		// Skip Primary Keys / Auto Increments
		if col.IsPrimaryKey || col.IsAutoIncr {
			continue
		}

		stats, err := a.driver.SampleColumnData(ctx, db, tableName, col.Name, 1000)
		if err != nil || stats == nil || stats.TotalSampled == 0 {
			continue
		}

		rec := a.evaluateColumn(tableName, col, stats)
		if rec != nil {
			recommendations = append(recommendations, *rec)
		}
	}

	return recommendations, nil
}

func (a *Advisor) evaluateColumn(tableName string, col database.ColumnInfo, stats *database.ColumnSampleStats) *DatatypeRecommendation {
	currUpper := strings.ToUpper(col.RawType)
	if currUpper == "" {
		currUpper = strings.ToUpper(col.DataType)
	}

	// 1. UUID detected in VARCHAR or TEXT
	if stats.IsUUID && stats.MaxLength == 36 && (strings.Contains(currUpper, "VARCHAR") || strings.Contains(currUpper, "TEXT")) {
		return &DatatypeRecommendation{
			TableName:     tableName,
			ColumnName:    col.Name,
			CurrentType:   currUpper,
			SuggestedType: "CHAR(36) or UUID",
			Confidence:    95,
			ObservedStats: fmt.Sprintf("Sampled %d rows. All values match standard UUID regex (fixed length 36).", stats.TotalSampled),
			Rationale:     "UUIDs have a fixed length. Storing in fixed CHAR(36) or native UUID reduces variable-length overhead.",
			FutureWarning: "Ensure application will never store non-UUID identifier strings in this column.",
		}
	}

	// 2. Boolean stored as string or large integer
	if stats.IsBoolean && (strings.Contains(currUpper, "VARCHAR") || strings.Contains(currUpper, "INT") && !strings.Contains(currUpper, "TINYINT(1)") && !strings.Contains(currUpper, "BOOL")) {
		return &DatatypeRecommendation{
			TableName:     tableName,
			ColumnName:    col.Name,
			CurrentType:   currUpper,
			SuggestedType: "BOOLEAN or TINYINT(1)",
			Confidence:    90,
			ObservedStats: fmt.Sprintf("Sampled %d rows. Only boolean values ('true', 'false', '0', '1') observed.", stats.TotalSampled),
			Rationale:     "Replacing string/integer with native BOOLEAN or TINYINT(1) saves up to 7 bytes per row and enforces domain constraints.",
			FutureWarning: "Verify that third-party integrations do not rely on string representations.",
		}
	}

	// 3. Numbers stored as VARCHAR
	if stats.IsNumeric && (strings.Contains(currUpper, "VARCHAR") || strings.Contains(currUpper, "TEXT")) {
		suggested := "INT"
		if stats.MaxLength > 9 {
			suggested = "BIGINT"
		}
		return &DatatypeRecommendation{
			TableName:     tableName,
			ColumnName:    col.Name,
			CurrentType:   currUpper,
			SuggestedType: suggested,
			Confidence:    85,
			ObservedStats: fmt.Sprintf("Sampled %d rows. All non-null values are numeric digits (max length %d).", stats.TotalSampled, stats.MaxLength),
			Rationale:     fmt.Sprintf("Storing numeric values in %s enables mathematical comparisons, indexing efficiency, and halves storage space.", suggested),
			FutureWarning: "If leading zeros are meaningful (e.g. postal codes, phone numbers), retain string representation.",
		}
	}

	// 4. Oversized VARCHAR / TEXT
	if strings.Contains(currUpper, "TEXT") && stats.MaxLength < 200 {
		return &DatatypeRecommendation{
			TableName:     tableName,
			ColumnName:    col.Name,
			CurrentType:   currUpper,
			SuggestedType: fmt.Sprintf("VARCHAR(%d)", max(50, stats.MaxLength*2)),
			Confidence:    80,
			ObservedStats: fmt.Sprintf("Sampled %d rows. Max observed length is %d chars (avg: %.1f).", stats.TotalSampled, stats.MaxLength, stats.AvgLength),
			Rationale:     "TEXT columns are stored off-page in many engines, requiring extra pointer dereferencing and inhibiting in-memory temporary tables.",
			FutureWarning: "Verify that user-supplied input will never exceed the recommended VARCHAR bound.",
		}
	}

	if strings.Contains(currUpper, "VARCHAR(255)") && stats.MaxLength < 40 && stats.TotalSampled > 20 {
		suggestedWidth := max(32, stats.MaxLength*2)
		return &DatatypeRecommendation{
			TableName:     tableName,
			ColumnName:    col.Name,
			CurrentType:   currUpper,
			SuggestedType: fmt.Sprintf("VARCHAR(%d)", suggestedWidth),
			Confidence:    75,
			ObservedStats: fmt.Sprintf("Sampled %d rows. Max observed length is only %d chars.", stats.TotalSampled, stats.MaxLength),
			Rationale:     fmt.Sprintf("Right-sizing VARCHAR(255) to VARCHAR(%d) reduces memory buffers allocated by query engines during sorting.", suggestedWidth),
			FutureWarning: "Allow buffer for unexpected future input growth before tightening limits.",
		}
	}

	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
