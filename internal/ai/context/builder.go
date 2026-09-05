package context

import (
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
)

// BuildMinifiedSchema generates a concise representation of database tables and relations
func BuildMinifiedSchema(tables []database.TableInfo, details map[string]*database.TableDetail) string {
	var sb strings.Builder
	sb.WriteString("DATABASE SCHEMA:\n")

	for _, t := range tables {
		detail, hasDetail := details[t.Name]
		if !hasDetail || detail == nil {
			sb.WriteString(fmt.Sprintf("Table: %s (Rows: %d)\n", t.Name, t.RowCount))
			continue
		}

		var colDefs []string
		for _, c := range detail.Columns {
			flags := ""
			if c.IsPrimaryKey {
				flags += " [PK]"
			}
			if c.IsForeignKey {
				flags += " [FK]"
			}
			colDefs = append(colDefs, fmt.Sprintf("%s %s%s", c.Name, c.RawType, flags))
		}

		sb.WriteString(fmt.Sprintf("Table: %s (Rows: %d)\n  Columns: %s\n", t.Name, detail.Table.RowCount, strings.Join(colDefs, ", ")))

		if len(detail.Indexes) > 0 {
			var idxDefs []string
			for _, idx := range detail.Indexes {
				if !idx.IsPrimary {
					idxDefs = append(idxDefs, fmt.Sprintf("%s(%s)", idx.Name, strings.Join(idx.Columns, ",")))
				}
			}
			if len(idxDefs) > 0 {
				sb.WriteString(fmt.Sprintf("  Indexes: %s\n", strings.Join(idxDefs, ", ")))
			}
		}

		if len(detail.ForeignKeys) > 0 {
			var fkDefs []string
			for _, fk := range detail.ForeignKeys {
				fkDefs = append(fkDefs, fmt.Sprintf("%s -> %s(%s)", strings.Join(fk.Columns, ","), fk.RefTableName, strings.Join(fk.RefColumns, ",")))
			}
			sb.WriteString(fmt.Sprintf("  ForeignKeys: %s\n", strings.Join(fkDefs, ", ")))
		}
	}

	return sb.String()
}
