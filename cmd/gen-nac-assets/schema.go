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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Candidate is a single documentable config-object node found while walking
// a NaC JSON Schema subtree. Path is dot-separated (e.g.
// "apic.access_policies.vlan_pools"). Folder is the first path segment below
// the solution (e.g. "access_policies") — used to scope doc matching.
type Candidate struct {
	Path       string
	Folder     string
	ObjectName string
	Schema     json.RawMessage
}

// FetchSchema downloads and decodes the combined NaC JSON Schema document.
func FetchSchema(url string) (map[string]any, error) {
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch schema: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read schema body: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	return doc, nil
}

// ExtractSolution returns the schema subtree rooted at properties.<solution>.
func ExtractSolution(doc map[string]any, solution string) (map[string]any, error) {
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema has no top-level properties object")
	}
	node, ok := props[solution].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema has no properties.%s", solution)
	}
	return node, nil
}

// titleSuffixRe strips the auto-generated suffix schemastore appends to
// titles, e.g. "Vlan Pools (List - Object)" -> "Vlan Pools".
var titleSuffixRe = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// objectNameFromTitle derives a display name from an auto-generated schema
// title, falling back to a title-cased version of the last path segment.
func objectNameFromTitle(title, lastSegment string) string {
	if title != "" {
		return strings.TrimSpace(titleSuffixRe.ReplaceAllString(title, ""))
	}
	return titleCaseSegment(lastSegment)
}

func titleCaseSegment(seg string) string {
	parts := strings.Split(seg, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// unwrap returns the object-shaped node describing an array's elements (for
// type:"array" nodes) or the node itself otherwise, plus whether it carries
// a non-empty "properties" map.
func unwrap(node map[string]any) (inner map[string]any, hasProps bool) {
	if nodeType, _ := node["type"].(string); nodeType == "array" {
		if items, ok := node["items"].(map[string]any); ok {
			inner = items
		}
	}
	if inner == nil {
		inner = node
	}
	props, ok := inner["properties"].(map[string]any)
	return inner, ok && len(props) > 0
}

// WalkCandidates recursively walks a solution subtree collecting every node
// (at any depth) that describes a distinct config object, i.e. an object
// (bare, or wrapped in an array) with its own "properties" map. folderRoots
// are the immediate children of the solution node (e.g. "access_policies") —
// only these subtrees are walked, matching where docs/templates/<solution>/
// <folder>/ files live.
func WalkCandidates(solutionNode map[string]any, solution string, folders []string) []Candidate {
	var out []Candidate
	props, _ := solutionNode["properties"].(map[string]any)

	folderSet := map[string]bool{}
	for _, f := range folders {
		folderSet[f] = true
	}

	// Sort for deterministic output ordering.
	keys := make([]string, 0, len(props))
	for k := range props {
		if folderSet[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, folder := range keys {
		node, ok := props[folder].(map[string]any)
		if !ok {
			continue
		}
		walkNode(node, solution+"."+folder, folder, &out)
	}
	return out
}

func walkNode(node map[string]any, path, folder string, out *[]Candidate) {
	inner, hasProps := unwrap(node)
	if !hasProps {
		return
	}

	segs := strings.Split(path, ".")
	last := segs[len(segs)-1]
	title, _ := inner["title"].(string)

	schemaBytes, err := json.Marshal(inner)
	if err == nil {
		*out = append(*out, Candidate{
			Path:       path,
			Folder:     folder,
			ObjectName: objectNameFromTitle(title, last),
			Schema:     schemaBytes,
		})
	}

	childProps, _ := inner["properties"].(map[string]any)
	childKeys := make([]string, 0, len(childProps))
	for k := range childProps {
		childKeys = append(childKeys, k)
	}
	sort.Strings(childKeys)

	for _, k := range childKeys {
		child, ok := childProps[k].(map[string]any)
		if !ok {
			continue
		}
		walkNode(child, path+"."+k, folder, out)
	}
}
