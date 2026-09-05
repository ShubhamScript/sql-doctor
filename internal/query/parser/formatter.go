package parser

import (
	"regexp"
	"strings"
)

var majorKeywords = []string{
	"SELECT", "FROM", "WHERE", "GROUP BY", "HAVING", "ORDER BY",
	"LIMIT", "OFFSET", "INSERT INTO", "VALUES", "UPDATE", "SET", "DELETE FROM",
	"LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "CROSS JOIN", "FULL OUTER JOIN", "JOIN",
	"UNION ALL", "UNION",
}

var subKeywords = []string{
	"AND", "OR", "ON",
}

var allKeywordsToUppercase = []string{
	"select", "from", "where", "group by", "having", "order by", "limit", "offset",
	"insert into", "values", "update", "set", "delete from", "left join", "right join",
	"inner join", "cross join", "full outer join", "join", "union all", "union",
	"and", "or", "on", "as", "asc", "desc", "distinct", "in", "is", "null", "not",
	"like", "between", "case", "when", "then", "else", "end", "exists", "count",
	"sum", "avg", "min", "max", "coalesce", "cast",
}

// Formatter pretty-prints and formats SQL statements
type Formatter struct{}

func NewFormatter() *Formatter {
	return &Formatter{}
}

func (f *Formatter) Format(sql string) string {
	clean := strings.TrimSpace(sql)
	clean = strings.TrimSuffix(clean, ";")

	// 1. Normalize multiple spaces/newlines to single space
	spaceRe := regexp.MustCompile(`\s+`)
	normalized := spaceRe.ReplaceAllString(clean, " ")

	// 2. Uppercase SQL keywords (preserving quoted literals)
	var tokens []string
	var currentToken strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(normalized); i++ {
		b := normalized[i]
		if inQuote {
			currentToken.WriteByte(b)
			if b == quoteChar {
				inQuote = false
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			continue
		}

		if b == '\'' || b == '"' || b == '`' {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			inQuote = true
			quoteChar = b
			currentToken.WriteByte(b)
			continue
		}

		if b == ' ' || b == ',' || b == '(' || b == ')' {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			tokens = append(tokens, string(b))
			continue
		}

		currentToken.WriteByte(b)
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	// Uppercase unquoted keywords
	kwMap := make(map[string]string)
	for _, kw := range allKeywordsToUppercase {
		kwMap[kw] = strings.ToUpper(kw)
	}

	for i, t := range tokens {
		if strings.HasPrefix(t, "'") || strings.HasPrefix(t, "\"") || strings.HasPrefix(t, "`") {
			continue
		}
		if upper, ok := kwMap[strings.ToLower(t)]; ok {
			tokens[i] = upper
		}
	}

	joined := strings.Join(tokens, "")

	// 3. Line break and indent major clauses
	formatted := joined
	for _, kw := range majorKeywords {
		re := regexp.MustCompile(`(?i)\b` + kw + `\b`)
		formatted = re.ReplaceAllString(formatted, "\n"+kw)
	}

	for _, kw := range subKeywords {
		re := regexp.MustCompile(`(?i)\b` + kw + `\b`)
		formatted = re.ReplaceAllString(formatted, "\n  "+kw)
	}

	// 4. Clean up formatting lines
	lines := strings.Split(formatted, "\n")
	var resultLines []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if strings.TrimSpace(trimmed) != "" {
			resultLines = append(resultLines, trimmed)
		}
	}

	return strings.Join(resultLines, "\n") + ";"
}
