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

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xrash/smetrics"
)

// matchThreshold is the minimum Jaro-Winkler similarity for a doc file to be
// considered a match for a schema candidate.
const matchThreshold = 0.75

// MatchResult pairs a matched DocFile with its Candidate and similarity score.
type MatchResult struct {
	Doc       DocFile
	Candidate Candidate
	Score     float64
}

// MatchReport summarizes match outcomes for one product/solution generation run.
type MatchReport struct {
	Matched          []MatchResult
	UnmatchedDocs    []DocFile
	UnmatchedCandies []Candidate
	// CollidedDocs holds docs that scored a valid match but lost to another
	// doc claiming the same candidate path — logged rather than silently
	// overwriting that candidate's output doc file.
	CollidedDocs []MatchResult
}

// normalize collapses a schema-path segment or doc filename into a
// comparable token: lowercase, underscores as spaces, common list-noun
// suffixes ("es"/"s") stripped so singular/plural forms compare equal.
func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.TrimSpace(s)

	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = singularize(p)
	}
	return strings.Join(parts, " ")
}

func singularize(word string) string {
	switch {
	case strings.HasSuffix(word, "ies") && len(word) > 3:
		return word[:len(word)-3] + "y"
	case strings.HasSuffix(word, "ses") && len(word) > 3:
		return word[:len(word)-2]
	case strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") && len(word) > 1:
		return word[:len(word)-1]
	}
	return word
}

// docKey strips a known folder-abbreviation prefix from a doc filename
// (e.g. "ap_" in access_policies docs) before normalizing, since NaC repo
// doc filenames commonly prefix the folder's initials.
func docKey(folder, name string) string {
	abbrev := folderAbbrev(folder)
	if abbrev != "" {
		name = strings.TrimPrefix(name, abbrev+"_")
	}
	return normalize(name)
}

// folderAbbrev derives the leading-letter abbreviation NaC doc authors use
// as a filename prefix within a folder, e.g. "access_policies" -> "ap".
func folderAbbrev(folder string) string {
	parts := strings.Split(folder, "_")
	var b strings.Builder
	for _, p := range parts {
		if p != "" {
			b.WriteByte(p[0])
		}
	}
	return b.String()
}

// MatchDocsToCandidates matches each doc file to the best-scoring schema
// candidate within the same folder subtree. Matches below matchThreshold, and
// candidates/docs left over, are reported as unmatched rather than dropped
// silently — the caller still emits schema entries for every candidate.
func MatchDocsToCandidates(candidates []Candidate, docs []DocFile) MatchReport {
	var report MatchReport

	byFolder := map[string][]Candidate{}
	for _, c := range candidates {
		byFolder[c.Folder] = append(byFolder[c.Folder], c)
	}

	sortedDocs := append([]DocFile(nil), docs...)
	sort.Slice(sortedDocs, func(i, j int) bool { return sortedDocs[i].Path < sortedDocs[j].Path })

	// claimed tracks, per candidate path, the best-scoring match found so
	// far. Docs that lose a claim are demoted to CollidedDocs rather than
	// silently overwriting the winner's output doc file.
	claimed := map[string]MatchResult{}

	for _, doc := range sortedDocs {
		pool := byFolder[doc.Folder]
		key := docKey(doc.Folder, doc.Name)

		best := -1.0
		var bestCand Candidate
		found := false
		for _, cand := range pool {
			segs := strings.Split(cand.Path, ".")
			last := segs[len(segs)-1]
			score := smetrics.JaroWinkler(key, normalize(last), 0.7, 4)
			if nameScore := smetrics.JaroWinkler(key, normalize(cand.ObjectName), 0.7, 4); nameScore > score {
				score = nameScore
			}
			if score > best {
				best = score
				bestCand = cand
				found = true
			}
		}

		if !found || best < matchThreshold {
			report.UnmatchedDocs = append(report.UnmatchedDocs, doc)
			continue
		}

		result := MatchResult{Doc: doc, Candidate: bestCand, Score: best}
		if existing, ok := claimed[bestCand.Path]; ok {
			if best > existing.Score {
				claimed[bestCand.Path] = result
				report.CollidedDocs = append(report.CollidedDocs, existing)
			} else {
				report.CollidedDocs = append(report.CollidedDocs, result)
			}
			continue
		}
		claimed[bestCand.Path] = result
	}

	matchedCandidatePaths := map[string]bool{}
	for path, result := range claimed {
		report.Matched = append(report.Matched, result)
		matchedCandidatePaths[path] = true
	}
	sort.Slice(report.Matched, func(i, j int) bool { return report.Matched[i].Doc.Path < report.Matched[j].Doc.Path })
	sort.Slice(report.CollidedDocs, func(i, j int) bool { return report.CollidedDocs[i].Doc.Path < report.CollidedDocs[j].Doc.Path })

	for _, c := range candidates {
		if !matchedCandidatePaths[c.Path] {
			report.UnmatchedCandies = append(report.UnmatchedCandies, c)
		}
	}

	return report
}

// WriteReport renders a human-readable match summary.
func WriteReport(r MatchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "matched: %d, unmatched docs: %d, unmatched candidates: %d, collided docs: %d\n\n",
		len(r.Matched), len(r.UnmatchedDocs), len(r.UnmatchedCandies), len(r.CollidedDocs))

	fmt.Fprintln(&b, "-- matched --")
	for _, m := range r.Matched {
		fmt.Fprintf(&b, "%.2f  %s  ->  %s\n", m.Score, m.Doc.Path, m.Candidate.Path)
	}

	fmt.Fprintln(&b, "\n-- collided docs (lost to a higher-scoring doc for the same candidate) --")
	for _, m := range r.CollidedDocs {
		fmt.Fprintf(&b, "%.2f  %s  ->  %s\n", m.Score, m.Doc.Path, m.Candidate.Path)
	}

	fmt.Fprintln(&b, "\n-- unmatched docs (no schema candidate found) --")
	for _, d := range r.UnmatchedDocs {
		fmt.Fprintln(&b, d.Path)
	}

	fmt.Fprintln(&b, "\n-- unmatched schema candidates (no doc found) --")
	for _, c := range r.UnmatchedCandies {
		fmt.Fprintln(&b, c.Path)
	}

	return b.String()
}
