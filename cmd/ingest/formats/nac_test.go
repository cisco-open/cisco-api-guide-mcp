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

package formats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// minimalNACSchema returns a minimal --input JSON array of flattened NAC
// schema entries.
func minimalNACSchema(entries []nacSchemaEntry) []byte {
	b, _ := json.Marshal(entries)
	return b
}

func TestNACSchemaHandler_Parse_BasicEntry(t *testing.T) {
	input := minimalNACSchema([]nacSchemaEntry{
		{
			Path:       "apic.access_policies.vlan_pools",
			ObjectName: "VLAN Pool",
			Schema:     json.RawMessage(`{"type":"array","items":{"type":"object"}}`),
		},
	})

	h := &NACSchemaHandler{}
	paths, err := h.Parse("nac-aci", input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	p := paths[0]
	if p.ProductID != "nac-aci" {
		t.Errorf("expected ProductID %q, got %q", "nac-aci", p.ProductID)
	}
	if p.Path != "apic.access_policies.vlan_pools" {
		t.Errorf("unexpected path %q", p.Path)
	}
	if p.ObjectName != "VLAN Pool" {
		t.Errorf("expected ObjectName %q, got %q", "VLAN Pool", p.ObjectName)
	}
	if p.SourceFormat != "nac-schema" {
		t.Errorf("expected SourceFormat %q, got %q", "nac-schema", p.SourceFormat)
	}
	if p.GUILocation != "" {
		t.Errorf("expected empty GUILocation without aux doc, got %q", p.GUILocation)
	}
	if p.Description != "" {
		t.Errorf("expected empty Description without aux doc, got %q", p.Description)
	}
	if p.Examples != "[]" {
		t.Errorf("expected empty examples array without aux doc, got %q", p.Examples)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(p.Schema), &schema); err != nil {
		t.Errorf("Schema is not valid JSON: %v", err)
	}
	if schema["type"] != "array" {
		t.Errorf("expected schema type %q, got %v", "array", schema["type"])
	}
}

func TestNACSchemaHandler_Parse_SkipsEmptyPath(t *testing.T) {
	input := minimalNACSchema([]nacSchemaEntry{
		{Path: "", ObjectName: "Bogus"},
	})

	h := &NACSchemaHandler{}
	paths, err := h.Parse("nac-aci", input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths for empty-path entry, got %d", len(paths))
	}
}

func TestNACSchemaHandler_Parse_DefaultsSchemaToEmptyObject(t *testing.T) {
	input := []byte(`[{"path":"apic.tenants","object_name":"Tenant"}]`)

	h := &NACSchemaHandler{}
	paths, err := h.Parse("nac-aci", input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].Schema != "{}" {
		t.Errorf("expected default schema %q, got %q", "{}", paths[0].Schema)
	}
}

func TestNACSchemaHandler_Parse_InvalidJSON(t *testing.T) {
	h := &NACSchemaHandler{}
	_, err := h.Parse("nac-aci", []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
}

func TestNACSchemaHandler_Parse_WithAuxDocs(t *testing.T) {
	input := minimalNACSchema([]nacSchemaEntry{
		{
			Path:       "apic.access_policies.vlan_pools",
			ObjectName: "VLAN Pool",
			Schema:     json.RawMessage(`{"type":"array"}`),
		},
	})

	auxDir := t.TempDir()
	auxDoc := nacAuxDoc{
		ObjectName:  "VLAN Pool",
		GUILocation: "Fabric > Access Policies > Pools > VLAN",
		Description: "Defines a static or dynamic VLAN range.",
		Examples: []nacAuxExample{
			{
				Title:       "Static VLAN pool",
				YAML:        "apic:\n  access_policies:\n    vlan_pools:\n      - name: POOL1\n",
				Explanation: "Creates a static VLAN pool named POOL1.",
			},
		},
	}
	auxBytes, _ := json.Marshal(auxDoc)
	if err := os.WriteFile(filepath.Join(auxDir, "apic.access_policies.vlan_pools.json"), auxBytes, 0644); err != nil {
		t.Fatalf("write aux doc: %v", err)
	}

	h := &NACSchemaHandler{AuxDir: auxDir}
	paths, err := h.Parse("nac-aci", input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	p := paths[0]
	if p.GUILocation != "Fabric > Access Policies > Pools > VLAN" {
		t.Errorf("unexpected GUILocation %q", p.GUILocation)
	}
	if p.Description != "Defines a static or dynamic VLAN range." {
		t.Errorf("unexpected Description %q", p.Description)
	}

	var examples []nacAuxExample
	if err := json.Unmarshal([]byte(p.Examples), &examples); err != nil {
		t.Fatalf("Examples is not valid JSON: %v", err)
	}
	if len(examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(examples))
	}
	if examples[0].Title != "Static VLAN pool" {
		t.Errorf("unexpected example title %q", examples[0].Title)
	}
}

func TestNACSchemaHandler_Parse_AuxDocObjectNameOverride(t *testing.T) {
	input := minimalNACSchema([]nacSchemaEntry{
		{
			Path:       "apic.tenants",
			ObjectName: "Tenant",
			Schema:     json.RawMessage(`{"type":"object"}`),
		},
	})

	auxDir := t.TempDir()
	auxDoc := nacAuxDoc{ObjectName: "Tenant (Fabric Policy Owner)"}
	auxBytes, _ := json.Marshal(auxDoc)
	if err := os.WriteFile(filepath.Join(auxDir, "apic.tenants.json"), auxBytes, 0644); err != nil {
		t.Fatalf("write aux doc: %v", err)
	}

	h := &NACSchemaHandler{AuxDir: auxDir}
	paths, err := h.Parse("nac-aci", input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].ObjectName != "Tenant (Fabric Policy Owner)" {
		t.Errorf("expected aux doc to override object name, got %q", paths[0].ObjectName)
	}
}

func TestNACSchemaHandler_Parse_SkipsMalformedAuxDoc(t *testing.T) {
	input := minimalNACSchema([]nacSchemaEntry{
		{Path: "apic.tenants", ObjectName: "Tenant", Schema: json.RawMessage(`{"type":"object"}`)},
	})

	auxDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(auxDir, "apic.tenants.json"), []byte("not json"), 0644); err != nil {
		t.Fatalf("write aux doc: %v", err)
	}

	h := &NACSchemaHandler{AuxDir: auxDir}
	paths, err := h.Parse("nac-aci", input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].ObjectName != "Tenant" {
		t.Errorf("expected fallback to schema object name, got %q", paths[0].ObjectName)
	}
}
