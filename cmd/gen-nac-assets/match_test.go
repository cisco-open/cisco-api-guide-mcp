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

import "testing"

func TestNormalizeSingularizes(t *testing.T) {
	cases := map[string]string{
		"vlan_pools":         "vlan pool",
		"bfd_policies":       "bfd policy",
		"span_classes":       "span class",
		"interface_policies": "interface policy",
		"vlan-pool":          "vlan pool",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDocKeyStripsFolderAbbrev(t *testing.T) {
	if got := docKey("access_policies", "ap_fex_interface_profile"); got != "fex interface profile" {
		t.Errorf("docKey = %q, want %q", got, "fex interface profile")
	}
	if got := docKey("access_policies", "vlan_pool"); got != "vlan pool" {
		t.Errorf("docKey = %q, want %q", got, "vlan pool")
	}
}

func TestFolderAbbrev(t *testing.T) {
	if got := folderAbbrev("access_policies"); got != "ap" {
		t.Errorf("folderAbbrev = %q, want %q", got, "ap")
	}
	if got := folderAbbrev("interface_policies"); got != "ip" {
		t.Errorf("folderAbbrev = %q, want %q", got, "ip")
	}
}

func TestMatchDocsToCandidatesBasic(t *testing.T) {
	candidates := []Candidate{
		{Path: "apic.access_policies.vlan_pools", Folder: "access_policies", ObjectName: "Vlan Pools"},
		{Path: "apic.access_policies.interface_policies.bfd_policies", Folder: "access_policies", ObjectName: "Bfd Policies"},
		{Path: "apic.access_policies.interface_policies.leaf_something", Folder: "access_policies", ObjectName: "Leaf Something"},
	}
	docs := []DocFile{
		{Path: "docs/templates/apic/access_policies/vlan_pool.md", Folder: "access_policies", Name: "vlan_pool"},
		{Path: "docs/templates/apic/access_policies/bfd_policy.md", Folder: "access_policies", Name: "bfd_policy"},
		{Path: "docs/templates/apic/access_policies/totally_unrelated_thing.md", Folder: "access_policies", Name: "totally_unrelated_thing"},
	}

	report := MatchDocsToCandidates(candidates, docs)

	if len(report.Matched) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(report.Matched), report.Matched)
	}
	byDocName := map[string]MatchResult{}
	for _, m := range report.Matched {
		byDocName[m.Doc.Name] = m
	}
	if m, ok := byDocName["vlan_pool"]; !ok || m.Candidate.Path != "apic.access_policies.vlan_pools" {
		t.Errorf("expected vlan_pool -> apic.access_policies.vlan_pools, got %+v", byDocName["vlan_pool"])
	}
	if m, ok := byDocName["bfd_policy"]; !ok || m.Candidate.Path != "apic.access_policies.interface_policies.bfd_policies" {
		t.Errorf("expected bfd_policy -> ...bfd_policies, got %+v", byDocName["bfd_policy"])
	}

	if len(report.UnmatchedDocs) != 1 || report.UnmatchedDocs[0].Name != "totally_unrelated_thing" {
		t.Errorf("expected totally_unrelated_thing unmatched, got %+v", report.UnmatchedDocs)
	}
	if len(report.UnmatchedCandies) != 1 || report.UnmatchedCandies[0].Path != "apic.access_policies.interface_policies.leaf_something" {
		t.Errorf("expected leaf_something candidate unmatched, got %+v", report.UnmatchedCandies)
	}
}

func TestMatchDocsToCandidatesNeverErrorsOnEmptyPool(t *testing.T) {
	docs := []DocFile{
		{Path: "docs/templates/apic/orphan_folder/thing.md", Folder: "orphan_folder", Name: "thing"},
	}
	report := MatchDocsToCandidates(nil, docs)

	if len(report.Matched) != 0 {
		t.Fatalf("expected no matches, got %+v", report.Matched)
	}
	if len(report.UnmatchedDocs) != 1 {
		t.Fatalf("expected doc reported unmatched, got %+v", report.UnmatchedDocs)
	}
}

func TestMatchDocsToCandidatesCollisionKeepsBestScore(t *testing.T) {
	candidates := []Candidate{
		{Path: "apic.fabric_policies.span", Folder: "fabric_policies", ObjectName: "Span"},
	}
	docs := []DocFile{
		{Path: "docs/templates/apic/fabric_policies/span.md", Folder: "fabric_policies", Name: "span"},
		{Path: "docs/templates/apic/fabric_policies/span_alt.md", Folder: "fabric_policies", Name: "span_alt"},
	}

	report := MatchDocsToCandidates(candidates, docs)

	if len(report.Matched) != 1 {
		t.Fatalf("expected 1 winning match, got %d: %+v", len(report.Matched), report.Matched)
	}
	if report.Matched[0].Doc.Name != "span" {
		t.Errorf("expected exact-name doc 'span' to win the collision, got %q", report.Matched[0].Doc.Name)
	}
	if len(report.CollidedDocs) != 1 || report.CollidedDocs[0].Doc.Name != "span_alt" {
		t.Errorf("expected span_alt demoted to CollidedDocs, got %+v", report.CollidedDocs)
	}
	if len(report.UnmatchedCandies) != 0 {
		t.Errorf("candidate has a matched doc, must not appear as unmatched: %+v", report.UnmatchedCandies)
	}
}

func TestWriteReportIncludesAllSections(t *testing.T) {
	report := MatchReport{
		Matched: []MatchResult{
			{Doc: DocFile{Path: "docs/templates/apic/access_policies/vlan_pool.md"}, Candidate: Candidate{Path: "apic.access_policies.vlan_pools"}, Score: 0.91},
		},
		UnmatchedDocs:    []DocFile{{Path: "docs/templates/apic/access_policies/mystery.md"}},
		UnmatchedCandies: []Candidate{{Path: "apic.access_policies.orphan_candidate"}},
	}

	out := WriteReport(report)
	for _, want := range []string{
		"matched: 1",
		"docs/templates/apic/access_policies/vlan_pool.md",
		"apic.access_policies.vlan_pools",
		"docs/templates/apic/access_policies/mystery.md",
		"apic.access_policies.orphan_candidate",
	} {
		if !contains(out, want) {
			t.Errorf("expected report to contain %q, got:\n%s", want, out)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
