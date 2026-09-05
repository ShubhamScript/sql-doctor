package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
)

// QualityAnomaly specifies an observed data anomaly
type QualityAnomaly struct {
	Level       string `json:"level"` // CRITICAL, WARNING, INFO
	Column      string `json:"column"`
	AnomalyType string `json:"anomaly_type"`
	Description string `json:"description"`
	Count       int64  `json:"impacted_count"`
	Sample      string `json:"sample_value,omitempty"`
}

// TableQualityReport summarizes the data cleanliness of a table
type TableQualityReport struct {
	TableName    string           `json:"table_name"`
	TotalSampled int64            `json:"total_sampled"`
	QualityScore int              `json:"quality_score"` // 0 - 100
	Anomalies    []QualityAnomaly `json:"anomalies"`
}

// QualityAnalyzer analyzes data cleanliness and formatting integrity
type QualityAnalyzer struct {
	driver database.Driver
}

func NewQualityAnalyzer(driver database.Driver) *QualityAnalyzer {
	return &QualityAnalyzer{driver: driver}
}

func (q *QualityAnalyzer) AnalyzeTable(ctx context.Context, db *sql.DB, tableName string) (*TableQualityReport, error) {
	detail, err := q.driver.DescribeTable(ctx, db, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table '%s': %w", tableName, err)
	}

	report := &TableQualityReport{
		TableName:    tableName,
		QualityScore: 100,
	}

	for _, col := range detail.Columns {
		stats, err := q.driver.SampleColumnData(ctx, db, tableName, col.Name, 1000)
		if err != nil || stats == nil || stats.TotalSampled == 0 {
			continue
		}

		if stats.TotalSampled > report.TotalSampled {
			report.TotalSampled = stats.TotalSampled
		}

		// 1. All NULLs
		if stats.NullCount == stats.TotalSampled && stats.TotalSampled > 10 {
			report.Anomalies = append(report.Anomalies, QualityAnomaly{
				Level:       "WARNING",
				Column:      col.Name,
				AnomalyType: "100% NULL Values",
				Description: fmt.Sprintf("Column '%s' is 100%% NULL across all %d sampled rows.", col.Name, stats.TotalSampled),
				Count:       stats.NullCount,
			})
			report.QualityScore -= 10
		} else if stats.NullPercentage > 80.0 {
			report.Anomalies = append(report.Anomalies, QualityAnomaly{
				Level:       "INFO",
				Column:      col.Name,
				AnomalyType: "High NULL Percentage",
				Description: fmt.Sprintf("Column '%s' is %.1f%% NULL (%d/%d rows).", col.Name, stats.NullPercentage, stats.NullCount, stats.TotalSampled),
				Count:       stats.NullCount,
			})
			report.QualityScore -= 5
		}

		// 2. Empty String vs NULL confusion
		if strings.Contains(strings.ToUpper(col.RawType), "CHAR") || strings.Contains(strings.ToUpper(col.RawType), "TEXT") {
			var emptyCount int64
			emptyQuery := fmt.Sprintf("SELECT COUNT(1) FROM \"%s\" WHERE \"%s\" = ''", tableName, col.Name)
			if q.driver.Dialect() == database.DialectMySQL || q.driver.Dialect() == database.DialectMariaDB {
				emptyQuery = fmt.Sprintf("SELECT COUNT(1) FROM `%s` WHERE `%s` = ''", tableName, col.Name)
			}
			if err := db.QueryRowContext(ctx, emptyQuery).Scan(&emptyCount); err == nil && emptyCount > 0 && stats.NullCount > 0 {
				report.Anomalies = append(report.Anomalies, QualityAnomaly{
					Level:       "WARNING",
					Column:      col.Name,
					AnomalyType: "Mixed Empty Strings and NULLs",
					Description: fmt.Sprintf("Column '%s' contains both %d empty strings ('') and %d NULLs.", col.Name, emptyCount, stats.NullCount),
					Count:       emptyCount + stats.NullCount,
				})
				report.QualityScore -= 5
			}
		}

		// 3. Duplicate Values on Unique/Candidate Identifier Columns
		colLower := strings.ToLower(col.Name)
		if (strings.HasSuffix(colLower, "_id") || strings.HasSuffix(colLower, "uuid") || strings.HasSuffix(colLower, "email") || strings.HasSuffix(colLower, "code")) && !col.IsPrimaryKey {
			var dupCount int64
			dupQuery := fmt.Sprintf("SELECT COUNT(1) FROM (SELECT \"%s\" FROM \"%s\" WHERE \"%s\" IS NOT NULL GROUP BY \"%s\" HAVING COUNT(1) > 1) AS d", col.Name, tableName, col.Name, col.Name)
			if q.driver.Dialect() == database.DialectMySQL || q.driver.Dialect() == database.DialectMariaDB {
				dupQuery = fmt.Sprintf("SELECT COUNT(1) FROM (SELECT `%s` FROM `%s` WHERE `%s` IS NOT NULL GROUP BY `%s` HAVING COUNT(1) > 1) AS d", col.Name, tableName, col.Name, col.Name)
			}
			if err := db.QueryRowContext(ctx, dupQuery).Scan(&dupCount); err == nil && dupCount > 0 {
				report.Anomalies = append(report.Anomalies, QualityAnomaly{
					Level:       "CRITICAL",
					Column:      col.Name,
					AnomalyType: "Duplicate Candidate Key Values",
					Description: fmt.Sprintf("Column '%s' contains %d duplicate distinct values despite resembling an identifier.", col.Name, dupCount),
					Count:       dupCount,
				})
				report.QualityScore -= 15
			}
		}

		// 4. Inconsistent Casing in Categorical String Columns
		if (strings.Contains(colLower, "status") || strings.Contains(colLower, "type") || strings.Contains(colLower, "state") || strings.Contains(colLower, "role")) && stats.DistinctCount > 1 && stats.DistinctCount < 20 {
			var mixedCasingCount int64
			casingQuery := fmt.Sprintf("SELECT COUNT(1) FROM (SELECT LOWER(\"%s\") FROM \"%s\" WHERE \"%s\" IS NOT NULL GROUP BY LOWER(\"%s\") HAVING COUNT(DISTINCT \"%s\") > 1) AS c", col.Name, tableName, col.Name, col.Name, col.Name)
			if q.driver.Dialect() == database.DialectMySQL || q.driver.Dialect() == database.DialectMariaDB {
				casingQuery = fmt.Sprintf("SELECT COUNT(1) FROM (SELECT LOWER(`%s`) FROM `%s` WHERE `%s` IS NOT NULL GROUP BY LOWER(`%s`) HAVING COUNT(DISTINCT `%s`) > 1) AS c", col.Name, tableName, col.Name, col.Name, col.Name)
			}
			if err := db.QueryRowContext(ctx, casingQuery).Scan(&mixedCasingCount); err == nil && mixedCasingCount > 0 {
				report.Anomalies = append(report.Anomalies, QualityAnomaly{
					Level:       "WARNING",
					Column:      col.Name,
					AnomalyType: "Inconsistent Categorical Casing",
					Description: fmt.Sprintf("Column '%s' has values that differ only by letter case (e.g. 'active' vs 'Active').", col.Name),
					Count:       mixedCasingCount,
				})
				report.QualityScore -= 5
			}
		}
	}

	if report.QualityScore < 0 {
		report.QualityScore = 0
	}

	return report, nil
}
