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
	"net/http"
	"net/http/httptest"
	"testing"
)

const testSchemaJSON = `{
  "properties": {
    "apic": {
      "properties": {
        "access_policies": {
          "properties": {
            "vlan_pools": {
              "type": "array",
              "items": {
                "title": "Vlan Pools (List - Object)",
                "type": "object",
                "properties": {
                  "name": {"type": "string"}
                }
              }
            },
            "interface_policies": {
              "type": "object",
              "title": "Interface Policies (Object)",
              "properties": {
                "bfd_policies": {
                  "type": "array",
                  "items": {
                    "title": "Bfd Policies (List - Object)",
                    "type": "object",
                    "properties": {
                      "name": {"type": "string"}
                    }
                  }
                },
                "leaf_flag": {"type": "boolean"}
              }
            }
          }
        }
      }
    }
  }
}`

func TestFetchSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testSchemaJSON))
	}))
	defer srv.Close()

	doc, err := FetchSchema(srv.URL)
	if err != nil {
		t.Fatalf("FetchSchema: %v", err)
	}
	if _, ok := doc["properties"]; !ok {
		t.Fatalf("expected top-level properties key, got %v", doc)
	}
}

func TestFetchSchemaHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := FetchSchema(srv.URL); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestExtractSolution(t *testing.T) {
	doc, err := FetchSchema(mustServe(t, testSchemaJSON))
	if err != nil {
		t.Fatalf("FetchSchema: %v", err)
	}

	node, err := ExtractSolution(doc, "apic")
	if err != nil {
		t.Fatalf("ExtractSolution: %v", err)
	}
	if _, ok := node["properties"]; !ok {
		t.Fatalf("expected apic node to have properties, got %v", node)
	}

	if _, err := ExtractSolution(doc, "ndo"); err == nil {
		t.Fatal("expected error for missing solution")
	}
}

func mustServe(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestWalkCandidatesRecursesNestedFolders(t *testing.T) {
	doc, err := FetchSchema(mustServe(t, testSchemaJSON))
	if err != nil {
		t.Fatalf("FetchSchema: %v", err)
	}
	apic, err := ExtractSolution(doc, "apic")
	if err != nil {
		t.Fatalf("ExtractSolution: %v", err)
	}

	candidates := WalkCandidates(apic, "apic", []string{"access_policies"})

	byPath := map[string]Candidate{}
	for _, c := range candidates {
		byPath[c.Path] = c
	}

	if _, ok := byPath["apic.access_policies.vlan_pools"]; !ok {
		t.Errorf("expected top-level array candidate apic.access_policies.vlan_pools, got %v", byPath)
	}
	if _, ok := byPath["apic.access_policies.interface_policies"]; !ok {
		t.Errorf("expected bucket-object candidate apic.access_policies.interface_policies, got %v", byPath)
	}
	nested, ok := byPath["apic.access_policies.interface_policies.bfd_policies"]
	if !ok {
		t.Fatalf("expected nested candidate apic.access_policies.interface_policies.bfd_policies, got %v", byPath)
	}
	if nested.ObjectName != "Bfd Policies" {
		t.Errorf("expected title suffix stripped, got %q", nested.ObjectName)
	}
	if nested.Folder != "access_policies" {
		t.Errorf("expected folder access_policies, got %q", nested.Folder)
	}

	if _, ok := byPath["apic.access_policies.interface_policies.leaf_flag"]; ok {
		t.Error("leaf_flag has no properties map and must not be a candidate")
	}
}

func TestObjectNameFromTitleFallback(t *testing.T) {
	if got := objectNameFromTitle("", "vlan_pools"); got != "Vlan Pools" {
		t.Errorf("expected title-cased fallback, got %q", got)
	}
	if got := objectNameFromTitle("Vlan Pools (List - Object)", "vlan_pools"); got != "Vlan Pools" {
		t.Errorf("expected suffix stripped, got %q", got)
	}
}
