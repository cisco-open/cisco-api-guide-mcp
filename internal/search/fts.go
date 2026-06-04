package search

import (
	"strings"
)

// BuildFTSQuery expands query terms using synonym map and builds FTS5 query string.
func BuildFTSQuery(input string, synonyms map[string]string) string {
	tokens := strings.Fields(strings.ToLower(input))
	var parts []string

	for _, tok := range tokens {
		if exp, ok := synonyms[tok]; ok {
			// Original token OR each expansion term
			expTerms := strings.Fields(exp)
			alternatives := []string{tok}
			alternatives = append(alternatives, expTerms...)
			// Quote multi-word terms; single words bare
			var quoted []string
			for _, a := range alternatives {
				if strings.Contains(a, " ") {
					quoted = append(quoted, `"`+a+`"`)
				} else {
					quoted = append(quoted, a)
				}
			}
			parts = append(parts, "("+strings.Join(quoted, " OR ")+")")
		} else {
			parts = append(parts, tok)
		}
	}

	return strings.Join(parts, " ")
}
