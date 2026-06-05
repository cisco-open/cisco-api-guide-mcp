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
	"strings"
	"testing"
)

// ---- splitClassName ---------------------------------------------------------

func TestSplitClassName_Standard(t *testing.T) {
	cases := []struct {
		input   string
		wantPkg string
		wantCls string
		wantOK  bool
	}{
		{"fvTenant", "fv", "Tenant", true},
		{"fvCtx", "fv", "Ctx", true},
		{"fvBD", "fv", "BD", true},
		{"fvAEPg", "fv", "AEPg", true},
		{"l3extOut", "l3ext", "Out", true},
		{"aaaAProvider", "aaa", "AProvider", true},
		{"vmmDomP", "vmm", "DomP", true},
	}
	for _, tc := range cases {
		pkg, cls, ok := splitClassName(tc.input)
		if ok != tc.wantOK {
			t.Errorf("splitClassName(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
			continue
		}
		if pkg != tc.wantPkg {
			t.Errorf("splitClassName(%q) pkg=%q, want %q", tc.input, pkg, tc.wantPkg)
		}
		if cls != tc.wantCls {
			t.Errorf("splitClassName(%q) cls=%q, want %q", tc.input, cls, tc.wantCls)
		}
	}
}

func TestSplitClassName_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"Tenant",       // starts with uppercase
		"alltlowercase", // no uppercase
		"123Foo",       // starts with digit
	}
	for _, name := range invalid {
		_, _, ok := splitClassName(name)
		if ok {
			t.Errorf("splitClassName(%q) expected ok=false", name)
		}
	}
}

// ---- buildACIClassDescription -----------------------------------------------

func TestBuildACIClassDescription_NoDNFormats(t *testing.T) {
	got := buildACIClassDescription("A policy owner.", nil)
	if got != "A policy owner." {
		t.Errorf("expected base description unchanged, got %q", got)
	}
}

func TestBuildACIClassDescription_EmptyDescription(t *testing.T) {
	dns := []string{"uni/tn-{name}"}
	got := buildACIClassDescription("", dns)
	if !strings.Contains(got, "DN formats:") {
		t.Error("expected 'DN formats:' header in output")
	}
	if !strings.Contains(got, "uni/tn-{name}") {
		t.Error("expected DN path in output")
	}
}

func TestBuildACIClassDescription_WithDescriptionAndDNs(t *testing.T) {
	dns := []string{"uni/tn-{name}", "uni/tn-{name}/ctx-{name}"}
	got := buildACIClassDescription("Base description.", dns)
	if !strings.HasPrefix(got, "Base description.") {
		t.Error("expected original description to appear first")
	}
	if !strings.Contains(got, "DN formats:") {
		t.Error("expected 'DN formats:' section")
	}
	for _, dn := range dns {
		if !strings.Contains(got, dn) {
			t.Errorf("expected DN %q in description", dn)
		}
	}
}

func TestBuildACIClassDescription_TruncatesLargeList(t *testing.T) {
	// Generate more than maxDNFormatsInDescription entries.
	dns := make([]string, maxDNFormatsInDescription+5)
	for i := range dns {
		dns[i] = "uni/tn-{name}/ep-" + string(rune('a'+i%26))
	}
	got := buildACIClassDescription("", dns)
	if !strings.Contains(got, "...and") {
		t.Error("expected truncation notice for large DN list")
	}
	// Exactly maxDNFormatsInDescription paths should appear as /api/mo/... lines.
	count := strings.Count(got, "/api/mo/")
	if count != maxDNFormatsInDescription {
		t.Errorf("expected %d DN paths in description, got %d", maxDNFormatsInDescription, count)
	}
}

// ---- ACIMetaHandler.Parse ---------------------------------------------------

// minimalMeta returns a minimal aci-meta.json byte slice with the given classes.
func minimalMeta(classes map[string]map[string]interface{}) []byte {
	data := map[string]interface{}{"classes": classes}
	b, _ := json.Marshal(data)
	return b
}

func TestACIMetaHandler_Parse_ClassEndpoint(t *testing.T) {
	meta := minimalMeta(map[string]map[string]interface{}{
		"fvTenant": {
			"isAbstract":     false,
			"isConfigurable": true,
			"properties":     map[string]interface{}{},
		},
	})

	h := &ACIMetaHandler{}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	var found bool
	for _, e := range endpoints {
		if e.Path == "/api/class/fvTenant.json" && e.Method == "GET" {
			found = true
			if e.ProductID != "aci" {
				t.Errorf("expected ProductID %q, got %q", "aci", e.ProductID)
			}
			if !strings.Contains(e.Summary, "fvTenant") {
				t.Errorf("expected summary to contain class name, got %q", e.Summary)
			}
			if e.SourceFormat != "aci-meta" {
				t.Errorf("expected SourceFormat %q, got %q", "aci-meta", e.SourceFormat)
			}
			// Tags should be ["fv"] for the fv package.
			var tags []string
			if err := json.Unmarshal([]byte(e.Tags), &tags); err != nil {
				t.Errorf("Tags is not valid JSON: %v", err)
			}
			if len(tags) != 1 || tags[0] != "fv" {
				t.Errorf("expected tags [\"fv\"], got %v", tags)
			}
		}
	}
	if !found {
		t.Error("expected GET /api/class/fvTenant.json endpoint, not found")
	}
}

func TestACIMetaHandler_Parse_SkipsAbstractClasses(t *testing.T) {
	meta := minimalMeta(map[string]map[string]interface{}{
		"fvTenant": {
			"isAbstract":     true, // should be excluded
			"isConfigurable": true,
			"properties":     map[string]interface{}{},
		},
	})

	h := &ACIMetaHandler{}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	for _, e := range endpoints {
		if strings.Contains(e.Path, "fvTenant") {
			t.Errorf("abstract class fvTenant should not produce any endpoints, got %s %s", e.Method, e.Path)
		}
	}
}

func TestACIMetaHandler_Parse_SkipsNonConfigurableClasses(t *testing.T) {
	meta := minimalMeta(map[string]map[string]interface{}{
		"fvTenant": {
			"isAbstract":     false,
			"isConfigurable": false, // should be excluded
			"properties":     map[string]interface{}{},
		},
	})

	h := &ACIMetaHandler{}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	for _, e := range endpoints {
		if strings.Contains(e.Path, "fvTenant") {
			t.Errorf("non-configurable class should not produce any endpoints, got %s %s", e.Method, e.Path)
		}
	}
}

func TestACIMetaHandler_Parse_SkipsInvalidClassNames(t *testing.T) {
	meta := minimalMeta(map[string]map[string]interface{}{
		"alltlowercase": { // no uppercase letter → invalid class name
			"isAbstract":     false,
			"isConfigurable": true,
			"properties":     map[string]interface{}{},
		},
	})

	h := &ACIMetaHandler{}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints for invalid class name, got %d", len(endpoints))
	}
}

func TestACIMetaHandler_Parse_WithAuxDocs_MOPathEndpoints(t *testing.T) {
	// Build a minimal aci-meta.json.
	meta := minimalMeta(map[string]map[string]interface{}{
		"fvTenant": {
			"isAbstract":     false,
			"isConfigurable": true,
			"properties": map[string]interface{}{
				"name":  map[string]interface{}{"isConfigurable": true},
				"descr": map[string]interface{}{"isConfigurable": true},
			},
		},
	})

	// Write an APIC per-class doc for fvTenant to a temp directory.
	auxDir := t.TempDir()
	auxDoc := map[string]interface{}{
		"fv:Tenant": map[string]interface{}{
			"label":   "Tenant",
			"comment": []string{"A policy owner in the virtual fabric."},
			"dnFormats": []string{
				"uni/tn-{name}",
			},
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"label":     "Name",
					"modelType": "naming:Name",
					"isNaming":  true,
					"mandatory": true,
				},
				"descr": map[string]interface{}{
					"label":     "Description",
					"modelType": "naming:Descr",
				},
			},
		},
	}
	auxBytes, _ := json.Marshal(auxDoc)
	if err := os.WriteFile(filepath.Join(auxDir, "fvTenant.json"), auxBytes, 0644); err != nil {
		t.Fatalf("write aux doc: %v", err)
	}

	h := &ACIMetaHandler{AuxDir: auxDir}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Expect: 1 class GET + 3 MO endpoints (GET/POST/DELETE for uni/tn-{name}).
	if len(endpoints) != 4 {
		t.Errorf("expected 4 endpoints (1 class + 3 MO), got %d", len(endpoints))
		for _, e := range endpoints {
			t.Logf("  %s %s", e.Method, e.Path)
		}
	}

	var hasMOGet, hasMOPost, hasMODelete bool
	for _, e := range endpoints {
		if e.Path == "/api/mo/uni/tn-{name}.json" {
			switch e.Method {
			case "GET":
				hasMOGet = true
			case "POST":
				hasMOPost = true
				// Verify the request body has the expected class structure.
				var rb struct {
					ClassName  string `json:"className"`
					Attributes []struct {
						Name     string `json:"name"`
						IsNaming bool   `json:"isNaming"`
					} `json:"attributes"`
				}
				if err := json.Unmarshal([]byte(e.RequestBody), &rb); err != nil {
					t.Errorf("POST request body is not valid JSON: %v", err)
					continue
				}
				if rb.ClassName != "fvTenant" {
					t.Errorf("expected className %q, got %q", "fvTenant", rb.ClassName)
				}
				// The "name" attribute should have isNaming=true.
				var hasNaming bool
				for _, attr := range rb.Attributes {
					if attr.Name == "name" && attr.IsNaming {
						hasNaming = true
					}
				}
				if !hasNaming {
					t.Error("expected 'name' attribute with isNaming=true in fvTenant request body")
				}
			case "DELETE":
				hasMODelete = true
			}
		}
		// The class endpoint description should embed DN format examples.
		if e.Path == "/api/class/fvTenant.json" {
			if !strings.Contains(e.Description, "DN formats:") {
				t.Error("class endpoint description should contain 'DN formats:' section")
			}
			if !strings.Contains(e.Description, "uni/tn-{name}") {
				t.Error("class endpoint description should contain the DN format path")
			}
			if !strings.Contains(e.Description, "A policy owner") {
				t.Error("class endpoint description should contain comment from aux doc")
			}
		}
	}
	if !hasMOGet {
		t.Error("expected GET /api/mo/uni/tn-{name}.json endpoint")
	}
	if !hasMOPost {
		t.Error("expected POST /api/mo/uni/tn-{name}.json endpoint")
	}
	if !hasMODelete {
		t.Error("expected DELETE /api/mo/uni/tn-{name}.json endpoint")
	}
}

func TestACIMetaHandler_Parse_WithAuxDocs_LabelOverridesSummary(t *testing.T) {
	meta := minimalMeta(map[string]map[string]interface{}{
		"fvTenant": {
			"isAbstract":     false,
			"isConfigurable": true,
			"properties":     map[string]interface{}{},
		},
	})

	auxDir := t.TempDir()
	auxDoc := map[string]interface{}{
		"fv:Tenant": map[string]interface{}{
			"label":      "Tenant",
			"comment":    []string{},
			"dnFormats":  []string{},
			"properties": map[string]interface{}{},
		},
	}
	auxBytes, _ := json.Marshal(auxDoc)
	os.WriteFile(filepath.Join(auxDir, "fvTenant.json"), auxBytes, 0644)

	h := &ACIMetaHandler{AuxDir: auxDir}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	for _, e := range endpoints {
		if e.Path == "/api/class/fvTenant.json" {
			// Summary should use the label "Tenant", not the raw class name.
			if !strings.Contains(e.Summary, "Tenant") {
				t.Errorf("expected summary to use label 'Tenant', got %q", e.Summary)
			}
		}
	}
}

func TestACIMetaHandler_Parse_MOPathsCappedAtMax(t *testing.T) {
	// Provide more DN formats than maxMOPathsPerClass.
	dns := make([]string, maxMOPathsPerClass+3)
	for i := range dns {
		// Each DN must be unique to avoid DB-level conflicts.
		dns[i] = "uni/tn-{name}/variant-" + string(rune('a'+i))
	}

	meta := minimalMeta(map[string]map[string]interface{}{
		"fvTenant": {
			"isAbstract":     false,
			"isConfigurable": true,
			"properties":     map[string]interface{}{},
		},
	})

	auxDir := t.TempDir()
	auxDoc := map[string]interface{}{
		"fv:Tenant": map[string]interface{}{
			"label":      "Tenant",
			"comment":    []string{},
			"dnFormats":  dns,
			"properties": map[string]interface{}{},
		},
	}
	auxBytes, _ := json.Marshal(auxDoc)
	os.WriteFile(filepath.Join(auxDir, "fvTenant.json"), auxBytes, 0644)

	h := &ACIMetaHandler{AuxDir: auxDir}
	endpoints, err := h.Parse("aci", meta)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// 1 class GET + maxMOPathsPerClass * 3 MO endpoints.
	wantTotal := 1 + maxMOPathsPerClass*3
	if len(endpoints) != wantTotal {
		t.Errorf("expected %d endpoints (1 class + %d*3 MO), got %d",
			wantTotal, maxMOPathsPerClass, len(endpoints))
	}
}
