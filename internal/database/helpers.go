package database

import "strings"

// IsNumericString checks if string represents a number
func IsNumericString(s string) bool {
	if s == "" {
		return false
	}
	dotCount := 0
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r == '.' {
			dotCount++
			if dotCount > 1 {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsBooleanString checks if string represents a boolean value
func IsBooleanString(s string) bool {
	lower := strings.ToLower(s)
	return lower == "true" || lower == "false" || lower == "1" || lower == "0" || lower == "t" || lower == "f" || lower == "yes" || lower == "no"
}
