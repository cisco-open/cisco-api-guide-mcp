// SPDX-License-Identifier: Apache-2.0

// Copyright 2026 Cisco Systems, Inc. and their affiliates

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
