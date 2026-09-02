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

// Package db_test provides integration tests for the db package.
//
// All tests are self-contained: newTestDB creates a fresh SQLite database,
// applies the production schema, and seeds it with representative endpoint
// data for all three supported platforms (ACI, NDFC, Intersight).  This
// keeps the suite portable — "go test ./..." always works without external
// files.
package db_test

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
	_ "github.com/ncruces/go-sqlite3/driver"
)

//go:embed schema.sql
var schemaSQL string

// ---- test helpers ----------------------------------------------------------

// newTestDB creates a temp SQLite file, applies the production schema, and
// seeds it with representative data for ACI, NDFC, and Intersight.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	f.Close()

	db, err := idb.OpenRW(f.Name())
	if err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	seedTestData(t, db)
	return db
}

func seedTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	// Products
	products := []struct{ id, name, desc, baseURL, authType, authNotes string }{
		{
			"aci",
			"Cisco ACI (APIC REST API)",
			"REST API for Cisco Application Centric Infrastructure (ACI). Uses a managed-object model; every resource is a managed object addressed by its distinguished name (DN).",
			"https://<apic>",
			"cookie",
			"Authenticate via POST /api/aaaLogin.json with {aaaUser:{attributes:{name,pwd}}}.",
		},
		{
			"ndfc",
			"Nexus Dashboard Fabric Controller (NDFC)",
			"REST API for Cisco NDFC (formerly DCNM). Manages data-centre fabric infrastructure, VXLANs, and network policies.",
			"https://<ndfc-host>",
			"bearer",
			"Obtain a token via POST /login with {userName, userPasswd, domain}.",
		},
		{
			"intersight",
			"Cisco Intersight",
			"REST API for Cisco Intersight, a SaaS platform for infrastructure management across data centre, edge, and cloud.",
			"https://intersight.com",
			"oauth2",
			"Intersight uses OAuth 2.0 client credentials or API key authentication.",
		},
	}
	for _, p := range products {
		if _, err := db.Exec(
			`INSERT INTO products(id,name,description,base_url,auth_type,auth_notes,auth_schema)
			 VALUES(?,?,?,?,?,?,'{}')`,
			p.id, p.name, p.desc, p.baseURL, p.authType, p.authNotes,
		); err != nil {
			t.Fatalf("seed product %q: %v", p.id, err)
		}
	}

	// Aliases
	aliases := []struct{ alias, productID string }{
		{"apic", "aci"},
		{"application centric infrastructure", "aci"},
		{"dcnm", "ndfc"},
		{"nexus dashboard fabric controller", "ndfc"},
	}
	for _, a := range aliases {
		if _, err := db.Exec(
			`INSERT INTO product_aliases(alias,product_id) VALUES(?,?)`,
			a.alias, a.productID,
		); err != nil {
			t.Fatalf("seed alias %q: %v", a.alias, err)
		}
	}

	// Synonyms
	if _, err := db.Exec(`INSERT INTO synonyms(term,expansion) VALUES(?,?)`,
		"tenant", "fvTenant"); err != nil {
		t.Fatalf("seed synonym: %v", err)
	}

	type endpoint struct {
		productID, release, method, path, summary, description, tags, params, reqBody, responses, format string
	}

	aciTenantDesc := "A policy owner in the virtual fabric. A tenant can be either a private or a shared entity.\n\nDN formats:\n  /api/mo/uni/tn-{name}.json"
	aciTenantReqBody := `{"className":"fvTenant","attributes":[` +
		`{"name":"name","label":"Name","type":"naming:Name","comment":"The name of the tenant.","isNaming":true},` +
		`{"name":"descr","label":"Description","type":"naming:Descr","comment":"The description of the tenant."},` +
		`{"name":"annotation","label":"Annotation","type":"mo:Annotation","comment":"User annotation."},` +
		`{"name":"nameAlias","label":"Display Name","type":"naming:NameAlias"}` +
		`]}`
	aciVRFDesc := "A Virtual Routing and Forwarding (VRF) domain.\n\nDN formats:\n  /api/mo/uni/tn-{name}/ctx-{name}.json"
	ndfcVRFParams := `[` +
		`{"description":"Name of the Fabric. Ex: MyFabric","example":"MyFabric","in":"path","name":"fabric-name","required":true,"schema":{"type":"string"}},` +
		`{"description":"Filter field. Ex: vrfId==50000","example":"vrfId==50000","in":"query","name":"filter","required":false,"schema":{"type":"string"}}` +
		`]`
	intersightServerResponses := `{"200":{"description":"List of 'server.Profile' resources for the given filter criteria"},"401":{"description":"Unauthorized"},"403":{"description":"Forbidden"},"404":{"description":"Resource Not Found"}}`

	endpoints := []endpoint{
		// --- ACI : class listing endpoints ---
		{"aci", "5.2", "GET", "/api/class/fvTenant.json",
			"List all Tenant objects", aciTenantDesc,
			`["fv"]`, "[]", "{}", "{}", "aci-meta"},

		{"aci", "5.2", "GET", "/api/class/fvCtx.json",
			"List all VRF objects", aciVRFDesc,
			`["fv"]`, "[]", "{}", "{}", "aci-meta"},

		{"aci", "5.2", "GET", "/api/class/fvBD.json",
			"List all Bridge Domain objects", "A bridge domain defines a unique Layer 2 forwarding boundary.\n\nDN formats:\n  /api/mo/uni/tn-{name}/BD-{name}.json",
			`["fv"]`, "[]", "{}", "{}", "aci-meta"},

		{"aci", "5.2", "GET", "/api/class/fvAEPg.json",
			"List all Application EPG objects", "A set of requirements for the application-level EPG instance.\n\nDN formats:\n  /api/mo/uni/tn-{name}/ap-{name}/epg-{name}.json",
			`["fv"]`, "[]", "{}", "{}", "aci-meta"},

		// --- ACI : MO (distinguished-name) endpoints for Tenant ---
		{"aci", "5.2", "GET", "/api/mo/uni/tn-{name}.json",
			"Get Tenant by distinguished name", "A policy owner in the virtual fabric.",
			`["fv"]`, "[]", "{}", "{}", "aci-meta"},

		{"aci", "5.2", "POST", "/api/mo/uni/tn-{name}.json",
			"Create or update Tenant", "A policy owner in the virtual fabric.",
			`["fv"]`, "[]", aciTenantReqBody, "{}", "aci-meta"},

		{"aci", "5.2", "DELETE", "/api/mo/uni/tn-{name}.json",
			"Delete Tenant by distinguished name", "A policy owner in the virtual fabric.",
			`["fv"]`, "[]", "{}", "{}", "aci-meta"},

		// --- NDFC ---
		{"ndfc", "3.2.2m", "GET", "/appcenter/cisco/ndfc/api/v1/lan-fabric/rest/control/fabrics",
			"List all the Fabrics", "Returns all fabrics managed by NDFC.",
			`["fabric"]`, "[]", "{}", `{"200":{"description":"OK"}}`, "openapi3"},

		{"ndfc", "3.2.2m", "GET", "/appcenter/cisco/ndfc/api/v1/lan-fabric/rest/top-down/fabrics/{fabric-name}/vrfs",
			"List VRFs", "Returns all VRFs within the specified fabric.",
			`["vrf"]`, ndfcVRFParams, "{}", `{"200":{"description":"OK"}}`, "openapi3"},

		{"ndfc", "3.2.2m", "GET", "/appcenter/cisco/ndfc/api/v1/lan-fabric/rest/top-down/fabrics/{fabric-name}/networks",
			"List Networks", "Returns all networks within the specified fabric.",
			`["network"]`, `[{"in":"path","name":"fabric-name","required":true}]`,
			"{}", `{"200":{"description":"OK"}}`, "openapi3"},

		// --- Intersight ---
		{"intersight", "1.0.11", "GET", "/api/v1/server/Profiles",
			"Read a 'server.Profile' resource.",
			"Returns a list of server.Profile resources matching the given filter.",
			`["server"]`,
			`[{"in":"query","name":"$filter","required":false,"schema":{"type":"string"}}]`,
			"{}", intersightServerResponses, "openapi3"},

		{"intersight", "1.0.11", "GET", "/api/v1/compute/PhysicalSummaries",
			"Read a 'compute.PhysicalSummary' resource.",
			"Returns a list of compute.PhysicalSummary resources.",
			`["compute"]`,
			`[{"in":"query","name":"$filter","required":false},{"in":"query","name":"$top","required":false}]`,
			"{}", `{"200":{"description":"OK"}}`, "openapi3"},
	}

	stmt, err := db.Prepare(
		`INSERT INTO endpoints(product_id,release,method,path,summary,description,tags,parameters,request_body,responses,source_format)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatalf("prepare insert: %v", err)
	}
	defer stmt.Close()

	for _, e := range endpoints {
		if _, err := stmt.Exec(e.productID, e.release, e.method, e.path,
			e.summary, e.description, e.tags, e.params, e.reqBody, e.responses, e.format,
		); err != nil {
			t.Fatalf("seed endpoint %s %s: %v", e.method, e.path, err)
		}
	}
}

// ---- ResolveProduct --------------------------------------------------------

func TestResolveProduct_DirectMatch(t *testing.T) {
	db := newTestDB(t)
	for _, id := range []string{"aci", "ndfc", "intersight"} {
		got, err := idb.ResolveProduct(db, id)
		if err != nil {
			t.Errorf("ResolveProduct(%q): unexpected error %v", id, err)
		}
		if got != id {
			t.Errorf("ResolveProduct(%q) = %q, want %q", id, got, id)
		}
	}
}

func TestResolveProduct_Alias_APIC_ResolvesToACI(t *testing.T) {
	db := newTestDB(t)
	got, err := idb.ResolveProduct(db, "apic")
	if err != nil {
		t.Fatalf("ResolveProduct(\"apic\"): %v", err)
	}
	if got != "aci" {
		t.Errorf("expected \"aci\", got %q", got)
	}
}

func TestResolveProduct_Alias_DCNM_ResolvesToNDFC(t *testing.T) {
	db := newTestDB(t)
	got, err := idb.ResolveProduct(db, "dcnm")
	if err != nil {
		t.Fatalf("ResolveProduct(\"dcnm\"): %v", err)
	}
	if got != "ndfc" {
		t.Errorf("expected \"ndfc\", got %q", got)
	}
}

func TestResolveProduct_UnknownSlug_ReturnsError(t *testing.T) {
	db := newTestDB(t)
	_, err := idb.ResolveProduct(db, "unknown-product-xyz")
	if err == nil {
		t.Error("expected error for unknown product, got nil")
	}
}

// ---- GetProduct ------------------------------------------------------------

func TestGetProduct_ACI(t *testing.T) {
	db := newTestDB(t)
	p, err := idb.GetProduct(db, "aci")
	if err != nil {
		t.Fatalf("GetProduct(\"aci\"): %v", err)
	}
	if p.ID != "aci" {
		t.Errorf("ID: got %q, want %q", p.ID, "aci")
	}
	if p.BaseURL != "https://<apic>" {
		t.Errorf("BaseURL: got %q, want %q", p.BaseURL, "https://<apic>")
	}
	if p.AuthType != "cookie" {
		t.Errorf("AuthType: got %q, want %q", p.AuthType, "cookie")
	}
	if !strings.Contains(p.Name, "ACI") {
		t.Errorf("Name should contain ACI, got %q", p.Name)
	}
}

func TestGetProduct_NDFC(t *testing.T) {
	db := newTestDB(t)
	p, err := idb.GetProduct(db, "ndfc")
	if err != nil {
		t.Fatalf("GetProduct(\"ndfc\"): %v", err)
	}
	if p.AuthType != "bearer" {
		t.Errorf("AuthType: got %q, want %q", p.AuthType, "bearer")
	}
	if p.BaseURL != "https://<ndfc-host>" {
		t.Errorf("BaseURL: got %q, want %q", p.BaseURL, "https://<ndfc-host>")
	}
}

func TestGetProduct_Intersight(t *testing.T) {
	db := newTestDB(t)
	p, err := idb.GetProduct(db, "intersight")
	if err != nil {
		t.Fatalf("GetProduct(\"intersight\"): %v", err)
	}
	if p.AuthType != "oauth2" {
		t.Errorf("AuthType: got %q, want %q", p.AuthType, "oauth2")
	}
	if p.BaseURL != "https://intersight.com" {
		t.Errorf("BaseURL: got %q, want %q", p.BaseURL, "https://intersight.com")
	}
}

func TestGetProduct_Unknown_ReturnsError(t *testing.T) {
	db := newTestDB(t)
	_, err := idb.GetProduct(db, "no-such-product")
	if err == nil {
		t.Error("expected error for unknown product, got nil")
	}
}

// ---- GetEndpoint : ACI -------------------------------------------------------

// TestGetEndpoint_ACI_fvTenant verifies that the ACI Tenant class endpoint
// can be retrieved and contains the expected structure.
func TestGetEndpoint_ACI_fvTenant(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "aci", "5.2", "GET", "/api/class/fvTenant.json")
	if err != nil {
		t.Fatalf("GetEndpoint fvTenant: %v", err)
	}

	if e.ProductID != "aci" {
		t.Errorf("ProductID: got %q, want %q", e.ProductID, "aci")
	}
	if !strings.Contains(e.Summary, "Tenant") {
		t.Errorf("Summary should mention 'Tenant', got %q", e.Summary)
	}
	if !strings.Contains(e.Description, "DN formats:") {
		t.Error("Description should contain 'DN formats:' section")
	}
	if !strings.Contains(e.Description, "uni/tn-{name}") {
		t.Error("Description should include the Tenant DN pattern uni/tn-{name}")
	}
	if e.SourceFormat != "aci-meta" {
		t.Errorf("SourceFormat: got %q, want %q", e.SourceFormat, "aci-meta")
	}

	var tags []string
	if err := json.Unmarshal([]byte(e.Tags), &tags); err != nil {
		t.Fatalf("Tags is not valid JSON: %v", err)
	}
	if len(tags) == 0 || tags[0] != "fv" {
		t.Errorf("expected tags [\"fv\"], got %v", tags)
	}
}

// TestGetEndpoint_ACI_fvCtx verifies that the ACI VRF (fvCtx) class endpoint
// can be retrieved and the summary identifies it as a VRF resource.
func TestGetEndpoint_ACI_fvCtx_VRF(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "aci", "5.2", "GET", "/api/class/fvCtx.json")
	if err != nil {
		t.Fatalf("GetEndpoint fvCtx: %v", err)
	}
	if !strings.Contains(e.Summary, "VRF") {
		t.Errorf("Summary should identify fvCtx as a VRF, got %q", e.Summary)
	}
	if !strings.Contains(e.Description, "DN formats:") {
		t.Error("fvCtx description should contain 'DN formats:' section")
	}
}

// TestGetEndpoint_ACI_fvBD verifies the ACI Bridge Domain class endpoint.
func TestGetEndpoint_ACI_fvBD_BridgeDomain(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "aci", "5.2", "GET", "/api/class/fvBD.json")
	if err != nil {
		t.Fatalf("GetEndpoint fvBD: %v", err)
	}
	if !strings.Contains(e.Summary, "Bridge Domain") {
		t.Errorf("Summary should identify fvBD as a Bridge Domain, got %q", e.Summary)
	}
}

// TestGetEndpoint_ACI_fvAEPg verifies the ACI Application EPG class endpoint.
func TestGetEndpoint_ACI_fvAEPg_EPG(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "aci", "5.2", "GET", "/api/class/fvAEPg.json")
	if err != nil {
		t.Fatalf("GetEndpoint fvAEPg: %v", err)
	}
	if !strings.Contains(e.Summary, "EPG") {
		t.Errorf("Summary should identify fvAEPg as an EPG, got %q", e.Summary)
	}
}

// TestGetEndpoint_ACI_TenantMOPost verifies that the ACI Tenant MO POST
// endpoint carries a well-formed request body describing fvTenant attributes.
// Specifically it checks for the className discriminator and that the naming
// attribute "name" (and the descriptive attribute "descr") are present.
func TestGetEndpoint_ACI_TenantMO_POST_RequestBody(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "aci", "5.2", "POST", "/api/mo/uni/tn-{name}.json")
	if err != nil {
		t.Fatalf("GetEndpoint Tenant MO POST: %v", err)
	}

	// Request body must be parseable JSON.
	var rb struct {
		ClassName  string `json:"className"`
		Attributes []struct {
			Name     string `json:"name"`
			Label    string `json:"label"`
			Type     string `json:"type"`
			IsNaming bool   `json:"isNaming"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(e.RequestBody), &rb); err != nil {
		t.Fatalf("RequestBody is not valid JSON: %v\nbody: %s", err, e.RequestBody)
	}

	if rb.ClassName != "fvTenant" {
		t.Errorf("expected className=%q, got %q", "fvTenant", rb.ClassName)
	}

	attrByName := make(map[string]bool)
	var namingAttr string
	for _, attr := range rb.Attributes {
		attrByName[attr.Name] = true
		if attr.IsNaming {
			namingAttr = attr.Name
		}
	}

	// The ACI object model requires a "name" attribute (the naming property).
	if !attrByName["name"] {
		t.Error("fvTenant request body should include 'name' attribute")
	}
	if namingAttr != "name" {
		t.Errorf("expected 'name' to be the naming attribute, got %q", namingAttr)
	}

	// "descr" is a universally present attribute on all fv MOs.
	if !attrByName["descr"] {
		t.Error("fvTenant request body should include 'descr' attribute")
	}
}

// ---- GetEndpoint : NDFC ----------------------------------------------------

// TestGetEndpoint_NDFC_Fabrics verifies that the NDFC fabric list endpoint exists.
func TestGetEndpoint_NDFC_Fabrics(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "ndfc", "3.2.2m", "GET",
		"/appcenter/cisco/ndfc/api/v1/lan-fabric/rest/control/fabrics")
	if err != nil {
		t.Fatalf("GetEndpoint NDFC fabrics: %v", err)
	}
	if !strings.Contains(strings.ToLower(e.Summary), "fabric") {
		t.Errorf("expected 'fabric' in summary, got %q", e.Summary)
	}
	if e.SourceFormat != "openapi3" {
		t.Errorf("SourceFormat: got %q, want %q", e.SourceFormat, "openapi3")
	}
}

// TestGetEndpoint_NDFC_VRFs verifies that the NDFC VRF list endpoint can be
// retrieved and that the fabric-name path parameter is marked as required.
func TestGetEndpoint_NDFC_VRFs_FabricNameRequired(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "ndfc", "3.2.2m", "GET",
		"/appcenter/cisco/ndfc/api/v1/lan-fabric/rest/top-down/fabrics/{fabric-name}/vrfs")
	if err != nil {
		t.Fatalf("GetEndpoint NDFC VRFs: %v", err)
	}
	if !strings.Contains(e.Summary, "VRF") {
		t.Errorf("expected 'VRF' in summary, got %q", e.Summary)
	}

	// The fabric-name path parameter must be marked as required.
	var params []map[string]interface{}
	if err := json.Unmarshal([]byte(e.Parameters), &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v", err)
	}
	var foundFabricName bool
	for _, p := range params {
		if p["name"] == "fabric-name" {
			foundFabricName = true
			if req, _ := p["required"].(bool); !req {
				t.Error("fabric-name path parameter should be marked required=true")
			}
			if p["in"] != "path" {
				t.Errorf("expected fabric-name in=path, got %v", p["in"])
			}
		}
	}
	if !foundFabricName {
		t.Error("expected fabric-name path parameter in NDFC VRF list endpoint")
	}
}

// TestGetEndpoint_NDFC_Networks verifies the NDFC networks list endpoint.
func TestGetEndpoint_NDFC_Networks(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "ndfc", "3.2.2m", "GET",
		"/appcenter/cisco/ndfc/api/v1/lan-fabric/rest/top-down/fabrics/{fabric-name}/networks")
	if err != nil {
		t.Fatalf("GetEndpoint NDFC networks: %v", err)
	}
	if !strings.Contains(strings.ToLower(e.Summary), "network") {
		t.Errorf("expected 'network' in summary, got %q", e.Summary)
	}
}

// ---- GetEndpoint : Intersight ----------------------------------------------

// TestGetEndpoint_Intersight_ServerProfiles verifies the Intersight server
// profiles endpoint exists and includes a 200 response.
func TestGetEndpoint_Intersight_ServerProfiles(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "intersight", "1.0.11", "GET", "/api/v1/server/Profiles")
	if err != nil {
		t.Fatalf("GetEndpoint Intersight server.Profile: %v", err)
	}
	if !strings.Contains(e.Summary, "server.Profile") {
		t.Errorf("expected 'server.Profile' in summary, got %q", e.Summary)
	}

	var responses map[string]interface{}
	if err := json.Unmarshal([]byte(e.Responses), &responses); err != nil {
		t.Fatalf("Responses is not valid JSON: %v", err)
	}
	if _, has200 := responses["200"]; !has200 {
		t.Error("expected 200 response code for Intersight server.Profile GET")
	}
}

// TestGetEndpoint_Intersight_ComputePhysicalSummaries verifies the Intersight
// compute physical summary endpoint.
func TestGetEndpoint_Intersight_ComputePhysicalSummaries(t *testing.T) {
	db := newTestDB(t)
	e, err := idb.GetEndpoint(db, "intersight", "1.0.11", "GET", "/api/v1/compute/PhysicalSummaries")
	if err != nil {
		t.Fatalf("GetEndpoint Intersight compute.PhysicalSummary: %v", err)
	}
	if !strings.Contains(e.Summary, "compute.PhysicalSummary") {
		t.Errorf("expected 'compute.PhysicalSummary' in summary, got %q", e.Summary)
	}
}

// ---- GetEndpoint : error cases --------------------------------------------

func TestGetEndpoint_NotFound_ReturnsError(t *testing.T) {
	db := newTestDB(t)
	_, err := idb.GetEndpoint(db, "aci", "5.2", "GET", "/api/class/doesNotExist.json")
	if err == nil {
		t.Error("expected error for nonexistent endpoint, got nil")
	}
}

func TestGetEndpoint_MultipleReleases_WithoutPrefix_ReturnsError(t *testing.T) {
	db := newTestDB(t)

	// Insert the same endpoint under two different releases.
	for _, rel := range []string{"4.0", "5.0"} {
		if _, err := db.Exec(
			`INSERT INTO endpoints(product_id,release,method,path,summary,description,tags,parameters,request_body,responses,source_format)
			 VALUES('aci',?,'GET','/api/class/fvAp.json','List Application Profiles','','["fv"]','[]','{}','{}','aci-meta')`,
			rel,
		); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	_, err := idb.GetEndpoint(db, "aci", "", "GET", "/api/class/fvAp.json")
	if err == nil {
		t.Error("expected error when multiple releases match and no prefix is provided")
	}
}

func TestGetEndpoint_ReleasePrefix_PicksCorrectVersion(t *testing.T) {
	db := newTestDB(t)

	for _, rel := range []string{"4.0", "5.0"} {
		if _, err := db.Exec(
			`INSERT INTO endpoints(product_id,release,method,path,summary,description,tags,parameters,request_body,responses,source_format)
			 VALUES('aci',?,'GET','/api/class/fvAp.json','List Application Profiles','','["fv"]','[]','{}','{}','aci-meta')`,
			rel,
		); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	e, err := idb.GetEndpoint(db, "aci", "4", "GET", "/api/class/fvAp.json")
	if err != nil {
		t.Fatalf("GetEndpoint with prefix 4: %v", err)
	}
	if e.Release != "4.0" {
		t.Errorf("expected release %q, got %q", "4.0", e.Release)
	}
}

// ---- SearchEndpoints -------------------------------------------------------

// TestSearchEndpoints_ACI_fvTenant checks that searching for the ACI Tenant
// class name surfaces the /api/class/fvTenant.json endpoint.
func TestSearchEndpoints_ACI_fvTenant(t *testing.T) {
	db := newTestDB(t)
	results, _, err := idb.SearchEndpoints(db, "fvtenant", "aci", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints fvtenant: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'fvtenant', got none")
	}
	var found bool
	for _, r := range results {
		if r.Path == "/api/class/fvTenant.json" {
			found = true
			if r.ProductID != "aci" {
				t.Errorf("expected ProductID aci, got %q", r.ProductID)
			}
		}
	}
	if !found {
		t.Error("expected /api/class/fvTenant.json in fvtenant search results")
	}
}

// TestSearchEndpoints_ACI_VRF checks that searching for "vrf" in ACI finds
// the fvCtx class (the VRF managed object).
func TestSearchEndpoints_ACI_VRF_FindsFvCtx(t *testing.T) {
	db := newTestDB(t)
	results, _, err := idb.SearchEndpoints(db, "vrf", "aci", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints vrf/aci: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'vrf' in aci, got none")
	}
	var found bool
	for _, r := range results {
		if r.Path == "/api/class/fvCtx.json" {
			found = true
		}
	}
	if !found {
		t.Error("expected /api/class/fvCtx.json in ACI VRF search results")
	}
}

// TestSearchEndpoints_NDFC_VRF checks that searching for "vrf" in NDFC returns
// the NDFC VRF list endpoint (not the ACI fvCtx endpoint).
func TestSearchEndpoints_NDFC_VRF(t *testing.T) {
	db := newTestDB(t)
	results, _, err := idb.SearchEndpoints(db, "vrf", "ndfc", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints vrf/ndfc: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'vrf' in ndfc, got none")
	}
	for _, r := range results {
		if r.ProductID != "ndfc" {
			t.Errorf("product filter should exclude non-ndfc results, got %q", r.ProductID)
		}
	}
	var found bool
	for _, r := range results {
		if strings.Contains(r.Path, "vrfs") {
			found = true
		}
	}
	if !found {
		t.Error("expected a VRF list path in NDFC VRF search results")
	}
}

// TestSearchEndpoints_NDFC_Fabric checks that "fabric" search in NDFC finds
// the NDFC fabric list endpoint.
func TestSearchEndpoints_NDFC_Fabric(t *testing.T) {
	db := newTestDB(t)
	results, _, err := idb.SearchEndpoints(db, "fabric", "ndfc", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints fabric/ndfc: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'fabric' in ndfc, got none")
	}
	var found bool
	for _, r := range results {
		if strings.Contains(r.Path, "fabrics") {
			found = true
		}
	}
	if !found {
		t.Error("expected a fabric path in NDFC fabric search results")
	}
}

// TestSearchEndpoints_Intersight_Server checks that Intersight server
// endpoints are surfaced by a "server" search.
func TestSearchEndpoints_Intersight_Server(t *testing.T) {
	db := newTestDB(t)
	results, _, err := idb.SearchEndpoints(db, "server", "intersight", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints server/intersight: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'server' in intersight, got none")
	}
	var found bool
	for _, r := range results {
		if strings.Contains(r.Path, "/server/") {
			found = true
		}
	}
	if !found {
		t.Error("expected an Intersight /server/ path in server search results")
	}
}

// TestSearchEndpoints_ProductFilter_IsolatesResults ensures that setting
// productID excludes endpoints from other products.
func TestSearchEndpoints_ProductFilter_IsolatesResults(t *testing.T) {
	db := newTestDB(t)
	// "vrf" returns results from both aci and ndfc; filtering by "aci" should
	// exclude ndfc results.
	results, _, err := idb.SearchEndpoints(db, "vrf", "aci", "", 50)
	if err != nil {
		t.Fatalf("SearchEndpoints vrf filtered to aci: %v", err)
	}
	for _, r := range results {
		if r.ProductID != "aci" {
			t.Errorf("product filter 'aci' leaked %q result: %s %s",
				r.ProductID, r.Method, r.Path)
		}
	}
}

// TestSearchEndpoints_LimitIsRespected checks that the limit parameter is honoured.
func TestSearchEndpoints_LimitIsRespected(t *testing.T) {
	db := newTestDB(t)
	// Ask for at most 2 results across all products.
	results, _, err := idb.SearchEndpoints(db, "list", "", "", 2)
	if err != nil {
		t.Fatalf("SearchEndpoints with limit: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

// TestSearchEndpoints_NoResults returns gracefully when nothing matches.
func TestSearchEndpoints_NoResults(t *testing.T) {
	db := newTestDB(t)
	results, _, err := idb.SearchEndpoints(db, "zzznomatchxxx", "", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints (no match): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for unmatchable query, got %d", len(results))
	}
}

// ---- GetSynonyms -----------------------------------------------------------

func TestGetSynonyms_ReturnsSeedData(t *testing.T) {
	db := newTestDB(t)
	syns, err := idb.GetSynonyms(db)
	if err != nil {
		t.Fatalf("GetSynonyms: %v", err)
	}
	exp, ok := syns["tenant"]
	if !ok {
		t.Fatal("expected synonym 'tenant' to be present")
	}
	if exp != "fvTenant" {
		t.Errorf("expected expansion %q, got %q", "fvTenant", exp)
	}
}

func TestGetSynonyms_EmptyTable(t *testing.T) {
	// Build a fresh DB without seeding synonyms.
	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	db, err := idb.OpenRW(f.Name())
	if err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}

	syns, err := idb.GetSynonyms(db)
	if err != nil {
		t.Fatalf("GetSynonyms on empty table: %v", err)
	}
	if len(syns) != 0 {
		t.Errorf("expected empty map, got %d entries", len(syns))
	}
}

// ---- db.Open ---------------------------------------------------------------

// TestOpen_EmbeddedDB verifies that db.Open can accept a valid SQLite file's
// bytes and return a readable connection.  We use the embedded api.db from the
// embeddb package (which carries the schema but no data rows).
func TestOpen_EmbeddedDB(t *testing.T) {
	// Write our own minimal SQLite file so the test does not depend on the
	// embedded binary blob changing.  We open an in-memory DB, apply the
	// schema, and dump it to bytes via the normal file path.
	f, err := os.CreateTemp(t.TempDir(), "embedded-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()

	rw, err := idb.OpenRW(f.Name())
	if err != nil {
		t.Fatalf("OpenRW for test blob: %v", err)
	}
	if _, err := rw.Exec(schemaSQL); err != nil {
		rw.Close()
		t.Fatalf("schema: %v", err)
	}
	rw.Close()

	blob, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}

	db, err := idb.Open(blob)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer db.Close()

	// Verify the connection is usable.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&count); err != nil {
		t.Fatalf("query after Open: %v", err)
	}
}
