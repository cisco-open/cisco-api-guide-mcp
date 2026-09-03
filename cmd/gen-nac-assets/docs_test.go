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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGHEServer(t *testing.T, treeJSON string, raw map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/netascode/nac-aci/git/trees/master", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(treeJSON))
	})
	mux.HandleFunc("/raw/netascode/nac-aci/master/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/raw/netascode/nac-aci/master/")
		body, ok := raw[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchDocTree(t *testing.T) {
	tree := `{
		"tree": [
			{"path": "docs/templates/apic/access_policies/vlan_pool.md", "type": "blob"},
			{"path": "docs/templates/apic/access_policies/ap_fex_interface_profile.md", "type": "blob"},
			{"path": "docs/templates/apic/access_policies", "type": "tree"},
			{"path": "docs/templates/ndo/other.md", "type": "blob"},
			{"path": "docs/templates/apic/README.md", "type": "blob"},
			{"path": "some/other/file.md", "type": "blob"}
		],
		"truncated": false
	}`
	srv := newGHEServer(t, tree, nil)

	docs, truncated, err := FetchDocTree(srv.URL, "netascode/nac-aci", "master", "apic", "")
	if err != nil {
		t.Fatalf("FetchDocTree: %v", err)
	}
	if truncated {
		t.Error("expected truncated=false")
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs (README.md at solution root skipped, ndo skipped, other repo path skipped), got %d: %v", len(docs), docs)
	}

	byName := map[string]DocFile{}
	for _, d := range docs {
		byName[d.Name] = d
	}
	if d, ok := byName["vlan_pool"]; !ok || d.Folder != "access_policies" {
		t.Errorf("expected vlan_pool in access_policies, got %v", byName)
	}
	if d, ok := byName["ap_fex_interface_profile"]; !ok || d.Folder != "access_policies" {
		t.Errorf("expected ap_fex_interface_profile in access_policies, got %v", byName)
	}
}

func TestFetchDocTreeTruncated(t *testing.T) {
	tree := `{"tree": [], "truncated": true}`
	srv := newGHEServer(t, tree, nil)

	_, truncated, err := FetchDocTree(srv.URL, "netascode/nac-aci", "master", "apic", "")
	if err != nil {
		t.Fatalf("FetchDocTree: %v", err)
	}
	if !truncated {
		t.Error("expected truncated=true to be surfaced")
	}
}

func TestFetchRaw(t *testing.T) {
	srv := newGHEServer(t, `{"tree":[]}`, map[string]string{
		"docs/templates/apic/access_policies/vlan_pool.md": "# Vlan Pool\n",
	})

	content, err := FetchRaw(srv.URL, "netascode/nac-aci", "master", "docs/templates/apic/access_policies/vlan_pool.md", "")
	if err != nil {
		t.Fatalf("FetchRaw: %v", err)
	}
	if content != "# Vlan Pool\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestFetchRawNotFound(t *testing.T) {
	srv := newGHEServer(t, `{"tree":[]}`, map[string]string{})

	if _, err := FetchRaw(srv.URL, "netascode/nac-aci", "master", "docs/templates/apic/missing.md", ""); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestGhRequestSendsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if _, err := ghRequest(&http.Client{}, srv.URL, "secret-token"); err != nil {
		t.Fatalf("ghRequest: %v", err)
	}
	if gotAuth != "token secret-token" {
		t.Errorf("expected Authorization header, got %q", gotAuth)
	}
}

const sampleDoc = `# Vlan Pool

VLAN pools define ranges of VLAN IDs.

Location in GUI:
Fabric > Access Policies > Pools > VLAN

{{ doc_gen }}

### Examples

Example-1: A vlan pool with a static range.

` + "```yaml" + `
apic:
  access_policies:
    vlan_pools:
      - name: POOL1
        encap_blocks:
          - range: [100, 200]
` + "```" + `

Example-2: A vlan pool with a dynamic range.

` + "```yaml" + `
apic:
  access_policies:
    vlan_pools:
      - name: POOL2
` + "```" + `
`

func TestParseMarkdown(t *testing.T) {
	doc := ParseMarkdown(sampleDoc)

	if doc.ObjectName != "Vlan Pool" {
		t.Errorf("expected ObjectName 'Vlan Pool', got %q", doc.ObjectName)
	}
	if !strings.Contains(doc.Description, "VLAN pools define ranges") {
		t.Errorf("expected description to include intro paragraph, got %q", doc.Description)
	}
	if doc.GUILocation != "Fabric > Access Policies > Pools > VLAN" {
		t.Errorf("unexpected GUI location: %q", doc.GUILocation)
	}
	if len(doc.Examples) != 2 {
		t.Fatalf("expected 2 examples, got %d: %+v", len(doc.Examples), doc.Examples)
	}
	if doc.Examples[0].Title != "Example-1" {
		t.Errorf("unexpected example title: %q", doc.Examples[0].Title)
	}
	if doc.Examples[0].Explanation != "A vlan pool with a static range." {
		t.Errorf("unexpected explanation: %q", doc.Examples[0].Explanation)
	}
	if !strings.Contains(doc.Examples[0].YAML, "name: POOL1") {
		t.Errorf("unexpected yaml: %q", doc.Examples[0].YAML)
	}
	if !strings.Contains(doc.Examples[1].YAML, "name: POOL2") {
		t.Errorf("unexpected yaml: %q", doc.Examples[1].YAML)
	}
}

func TestParseMarkdownNoGUILocation(t *testing.T) {
	doc := ParseMarkdown("# Some Object\n\nJust a description, no GUI line.\n")
	if doc.ObjectName != "Some Object" {
		t.Errorf("unexpected ObjectName: %q", doc.ObjectName)
	}
	if doc.GUILocation != "" {
		t.Errorf("expected empty GUILocation, got %q", doc.GUILocation)
	}
	if !strings.Contains(doc.Description, "Just a description") {
		t.Errorf("unexpected description: %q", doc.Description)
	}
}

func TestParsedDocMarshalsToNacAuxDocShape(t *testing.T) {
	doc := ParseMarkdown(sampleDoc)
	aux := nacAuxDoc{
		ObjectName:  doc.ObjectName,
		GUILocation: doc.GUILocation,
		Description: doc.Description,
		Examples:    doc.Examples,
	}
	b, err := json.Marshal(aux)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"object_name", "gui_location", "description", "examples"} {
		if _, ok := back[key]; !ok {
			t.Errorf("expected json key %q in marshaled nacAuxDoc, got %s", key, b)
		}
	}
}

func TestTruncateHelper(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("expected unchanged short string, got %q", got)
	}
	long := strings.Repeat("a", 20)
	if got := truncate(long, 5); got != fmt.Sprintf("%s…", long[:5]) {
		t.Errorf("unexpected truncation: %q", got)
	}
}
