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
	"strings"
	"time"
)

// DocFile is one docs/templates/<solution>/<folder>/<name>.md file located
// via the GHE tree listing.
type DocFile struct {
	Path   string // full repo path, e.g. docs/templates/apic/access_policies/vlan_pool.md
	Folder string // e.g. access_policies
	Name   string // filename without extension, e.g. vlan_pool
}

type gheTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type gheTreeResponse struct {
	Entries   []gheTreeEntry `json:"tree"`
	Truncated bool           `json:"truncated"`
}

// ghRequest performs an authenticated (if token != "") GET against the GHE
// API/raw host and returns the response body.
func ghRequest(client *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %s: %s", url, resp.Status, truncate(string(body), 200))
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FetchDocTree lists every docs/templates/<solution>/**/*.md file in repo@branch
// via the GHE recursive Git Trees API.
func FetchDocTree(baseURL, repo, branch, solution, token string) (docs []DocFile, truncated bool, err error) {
	client := &http.Client{Timeout: 60 * time.Second}
	url := fmt.Sprintf("%s/api/v3/repos/%s/git/trees/%s?recursive=1", baseURL, repo, branch)

	body, err := ghRequest(client, url, token)
	if err != nil {
		return nil, false, fmt.Errorf("fetch doc tree: %w", err)
	}

	var tree gheTreeResponse
	if err := json.Unmarshal(body, &tree); err != nil {
		return nil, false, fmt.Errorf("decode doc tree: %w", err)
	}

	prefix := fmt.Sprintf("docs/templates/%s/", solution)
	for _, e := range tree.Entries {
		if e.Type != "blob" || !strings.HasPrefix(e.Path, prefix) || !strings.HasSuffix(e.Path, ".md") {
			continue
		}
		rest := strings.TrimPrefix(e.Path, prefix) // <folder>/<name>.md
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			continue // not directly under a folder — skip
		}
		name := strings.TrimSuffix(parts[1], ".md")
		docs = append(docs, DocFile{Path: e.Path, Folder: parts[0], Name: name})
	}

	return docs, tree.Truncated, nil
}

// FetchRaw downloads the raw content of a single repo file.
func FetchRaw(baseURL, repo, branch, path, token string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	url := fmt.Sprintf("%s/raw/%s/%s/%s", baseURL, repo, branch, path)
	body, err := ghRequest(client, url, token)
	if err != nil {
		return "", fmt.Errorf("fetch raw %s: %w", path, err)
	}
	return string(body), nil
}

// ParsedDoc is the result of parsing one docs/templates markdown file.
type ParsedDoc struct {
	ObjectName  string
	GUILocation string
	Description string
	Examples    []nacAuxExample
}

type nacAuxExample struct {
	Title       string `json:"title"`
	YAML        string `json:"yaml"`
	Explanation string `json:"explanation"`
}

var (
	h1Re       = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	guiLabelRe = regexp.MustCompile(`(?im)^Location in GUI:\s*$`)
	exampleRe  = regexp.MustCompile("(?s)(Example-?\\d+)\\s*:\\s*(.*?)\\s*```yaml\\s*\\n(.*?)```")
)

// ParseMarkdown extracts title, GUI breadcrumb, intro description, and
// worked examples from a docs/templates/<solution>/<folder>/<object>.md file.
//
// Expected shape (see netascode/nac-aci docs/templates/apic/access_policies/
// vlan_pool.md): "# <Title>" heading, optional intro paragraph, a
// "Location in GUI:" label followed by a breadcrumb line, a {{ doc_gen }}
// placeholder (ignored), then "### Examples" with one or more
// "Example-N: <prose>" paragraphs each followed by a fenced ```yaml block.
func ParseMarkdown(content string) ParsedDoc {
	var doc ParsedDoc

	titleEnd := 0
	if m := h1Re.FindStringSubmatchIndex(content); m != nil {
		doc.ObjectName = strings.TrimSpace(content[m[2]:m[3]])
		titleEnd = m[1]
	}

	if loc := guiLabelRe.FindStringIndex(content); loc != nil {
		doc.Description = strings.TrimSpace(content[titleEnd:loc[0]])

		rest := content[loc[1]:]
		for line := range strings.SplitSeq(rest, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			doc.GUILocation = line
			break
		}
	} else {
		doc.Description = strings.TrimSpace(content[titleEnd:])
	}

	for _, m := range exampleRe.FindAllStringSubmatch(content, -1) {
		doc.Examples = append(doc.Examples, nacAuxExample{
			Title:       strings.TrimSpace(m[1]),
			Explanation: strings.TrimSpace(m[2]),
			YAML:        strings.TrimSpace(m[3]),
		})
	}

	return doc
}
