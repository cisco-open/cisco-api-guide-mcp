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

package modules_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
	"github.com/cisco-open/cisco-api-guide-mcp/internal/modules"
)

func createTestDB(t *testing.T, productID, name string, aliases []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, productID+".db")

	db, err := idb.OpenRW(dbPath)
	if err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	defer db.Close()

	schemaSQL, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO products(id, name, description, base_url, auth_type, auth_notes, auth_schema)
		VALUES(?, ?, 'Test Description', 'https://example.com', 'bearer', 'Test notes', '{}')`,
		productID, name); err != nil {
		t.Fatalf("insert product: %v", err)
	}

	for _, alias := range aliases {
		if _, err := db.Exec(`INSERT INTO product_aliases(alias, product_id) VALUES(?, ?)`, alias, productID); err != nil {
			t.Fatalf("insert alias: %v", err)
		}
	}

	if _, err := db.Exec(`
		INSERT INTO endpoints(product_id, release, method, path, summary, description, tags, parameters, request_body, responses, source_format)
		VALUES(?, '1.0', 'GET', '/api/v1/test', 'Test Summary', 'Test Description', '[]', '[]', '{}', '{}', 'openapi3')`,
		productID); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}

	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read created db: %v", err)
	}
	h := sha256.Sum256(dbBytes)
	hashStr := hex.EncodeToString(h[:])

	return dbPath, hashStr
}

func createTestNACDB(t *testing.T, productID, name string, aliases []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, productID+".db")

	db, err := idb.OpenRW(dbPath)
	if err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	defer db.Close()

	schemaSQL, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO products(id, name, description, base_url, auth_type, auth_notes, auth_schema)
		VALUES(?, ?, 'Test NaC Description', '', '', '', '{}')`,
		productID, name); err != nil {
		t.Fatalf("insert product: %v", err)
	}

	for _, alias := range aliases {
		if _, err := db.Exec(`INSERT INTO product_aliases(alias, product_id) VALUES(?, ?)`, alias, productID); err != nil {
			t.Fatalf("insert alias: %v", err)
		}
	}

	if _, err := db.Exec(`
		INSERT INTO nac_paths(product_id, release, path, object_name, gui_location, description, schema, examples, source_format)
		VALUES(?, '2.0.0', 'apic.access_policies.vlan_pools', 'VLAN Pool', 'Fabric > Access Policies > Pools > VLAN', 'Defines a static VLAN pool.', '{"type":"array"}', '[]', 'nac-schema')`,
		productID); err != nil {
		t.Fatalf("insert nac_path: %v", err)
	}

	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read created db: %v", err)
	}
	h := sha256.Sum256(dbBytes)
	hashStr := hex.EncodeToString(h[:])

	return dbPath, hashStr
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func TestModuleFetcher_DownloadAndLoad(t *testing.T) {
	aciPath, aciHash := createTestDB(t, "aci", "Cisco ACI", []string{"apic"})
	ndfcPath, ndfcHash := createTestDB(t, "ndfc", "Cisco NDFC", []string{"dcnm"})

	aciRaw, _ := os.ReadFile(aciPath)
	ndfcRaw, _ := os.ReadFile(ndfcPath)

	aciGz := gzipBytes(t, aciRaw)
	ndfcGz := gzipBytes(t, ndfcRaw)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/aci.db.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(aciGz)
	})

	mux.HandleFunc("/ndfc.db.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(ndfcGz)
	})

	manifest := modules.Manifest{
		Version: 1,
		Modules: map[string]modules.ModuleInfo{
			"aci": {
				Name:      "Cisco ACI",
				ProductID: "aci",
				Version:   "5.2",
				SHA256:    aciHash,
				URL:       server.URL + "/aci.db.gz",
				Aliases:   []string{"apic"},
			},
			"ndfc": {
				Name:      "Cisco NDFC",
				ProductID: "ndfc",
				Version:   "4.1.1",
				SHA256:    ndfcHash,
				URL:       server.URL + "/ndfc.db.gz",
				Aliases:   []string{"dcnm"},
			},
		},
	}

	manifestBytes, _ := json.Marshal(manifest)
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(manifestBytes)
	})

	dataDir := t.TempDir()
	fetcher, err := modules.NewModuleFetcher(modules.FetcherOptions{
		DataDir:     dataDir,
		RegistryURL: server.URL + "/manifest.json",
	})
	if err != nil {
		t.Fatalf("NewModuleFetcher: %v", err)
	}

	// 1. Ensure only aci module
	paths, err := fetcher.EnsureModules([]string{"aci"}, false)
	if err != nil {
		t.Fatalf("EnsureModules aci: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	// 2. Load into DB Manager
	mgr := idb.NewManager()
	defer mgr.Close()

	if err := fetcher.LoadIntoManager(mgr, paths); err != nil {
		t.Fatalf("LoadIntoManager: %v", err)
	}

	if mgr.ModuleCount() != 1 {
		t.Errorf("expected 1 loaded module, got %d", mgr.ModuleCount())
	}

	prod, err := mgr.GetProduct("aci")
	if err != nil {
		t.Fatalf("GetProduct aci: %v", err)
	}
	if prod.Name != "Cisco ACI" {
		t.Errorf("expected Cisco ACI, got %s", prod.Name)
	}

	// 3. Search endpoint
	results, count, err := mgr.SearchEndpoints("test", "aci", "", 10)
	if err != nil {
		t.Fatalf("SearchEndpoints: %v", err)
	}
	if count != 1 || len(results) != 1 {
		t.Fatalf("expected 1 search result, got count=%d len=%d", count, len(results))
	}

	// 4. Test alias resolution
	resolved, err := mgr.ResolveProduct("apic")
	if err != nil {
		t.Fatalf("ResolveProduct(apic): %v", err)
	}
	if resolved != "aci" {
		t.Errorf("expected aci, got %s", resolved)
	}
}

func TestModuleFetcher_DownloadAndLoad_NACProduct(t *testing.T) {
	nacPath, nacHash := createTestNACDB(t, "nac-aci", "Cisco NaC for ACI", []string{"nac-apic"})

	nacRaw, _ := os.ReadFile(nacPath)
	nacGz := gzipBytes(t, nacRaw)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/nac-aci.db.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(nacGz)
	})

	manifest := modules.Manifest{
		Version: 1,
		Modules: map[string]modules.ModuleInfo{
			"nac-aci": {
				Name:      "Cisco NaC for ACI",
				ProductID: "nac-aci",
				Version:   "2.0.0",
				SHA256:    nacHash,
				URL:       server.URL + "/nac-aci.db.gz",
				Aliases:   []string{"nac-apic"},
			},
		},
	}

	manifestBytes, _ := json.Marshal(manifest)
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(manifestBytes)
	})

	dataDir := t.TempDir()
	fetcher, err := modules.NewModuleFetcher(modules.FetcherOptions{
		DataDir:     dataDir,
		RegistryURL: server.URL + "/manifest.json",
	})
	if err != nil {
		t.Fatalf("NewModuleFetcher: %v", err)
	}

	paths, err := fetcher.EnsureModules([]string{"nac-aci"}, false)
	if err != nil {
		t.Fatalf("EnsureModules nac-aci: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	mgr := idb.NewManager()
	defer mgr.Close()

	if err := fetcher.LoadIntoManager(mgr, paths); err != nil {
		t.Fatalf("LoadIntoManager: %v", err)
	}

	if mgr.ModuleCount() != 1 {
		t.Errorf("expected 1 loaded module, got %d", mgr.ModuleCount())
	}

	prod, err := mgr.GetProduct("nac-aci")
	if err != nil {
		t.Fatalf("GetProduct nac-aci: %v", err)
	}
	if prod.Name != "Cisco NaC for ACI" {
		t.Errorf("expected Cisco NaC for ACI, got %s", prod.Name)
	}

	results, count, err := mgr.SearchNACPaths("vlan", "nac-aci", "", 10)
	if err != nil {
		t.Fatalf("SearchNACPaths: %v", err)
	}
	if count != 1 || len(results) != 1 {
		t.Fatalf("expected 1 search result, got count=%d len=%d", count, len(results))
	}
	if results[0].Path != "apic.access_policies.vlan_pools" {
		t.Errorf("expected apic.access_policies.vlan_pools, got %s", results[0].Path)
	}

	p, err := mgr.GetNACPath("nac-aci", "", "apic.access_policies.vlan_pools")
	if err != nil {
		t.Fatalf("GetNACPath: %v", err)
	}
	if p.ObjectName != "VLAN Pool" {
		t.Errorf("expected VLAN Pool, got %s", p.ObjectName)
	}

	resolved, err := mgr.ResolveProduct("nac-apic")
	if err != nil {
		t.Fatalf("ResolveProduct(nac-apic): %v", err)
	}
	if resolved != "nac-aci" {
		t.Errorf("expected nac-aci, got %s", resolved)
	}
}
