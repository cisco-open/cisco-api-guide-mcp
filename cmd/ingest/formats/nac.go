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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
)

// NACSchemaHandler ingests Network-as-Code (NaC) YAML configuration paths.
//
// --input   : path to a pre-flattened per-product NAC schema JSON file (an
//             array of nacSchemaEntry), derived from the public netascode
//             schemastore JSON split/flattened per product.
// --aux-dir : optional path to a directory of per-path JSON doc files
//             (one file per config path, e.g. apic.access_policies.vlan_pools.json)
//             extracted from the internal nac-<product> repo's
//             docs/templates/<solution>/<folder>/<object>.md markdown files.
//
// Without --aux-dir only the raw schema fragment is stored (structural data:
// type/required/enum/default/pattern). With --aux-dir the handler also
// attaches object name, GUI location, human description, and worked examples.
type NACSchemaHandler struct {
	AuxDir string // optional; set by main.go before Parse is called
}

// nacSchemaEntry is one flattened NAC config path entry in the --input file.
type nacSchemaEntry struct {
	Path       string          `json:"path"`
	ObjectName string          `json:"object_name"`
	Schema     json.RawMessage `json:"schema"`
}

// nacAuxDoc is the aux-dir doc file shape for one NAC config path, extracted
// from the internal nac-<product> repo's docs/templates markdown.
type nacAuxDoc struct {
	ObjectName  string          `json:"object_name"`
	GUILocation string          `json:"gui_location"`
	Description string          `json:"description"`
	Examples    []nacAuxExample `json:"examples"`
}

type nacAuxExample struct {
	Title       string `json:"title"`
	YAML        string `json:"yaml"`
	Explanation string `json:"explanation"`
}

func (h *NACSchemaHandler) Parse(productID string, data []byte) ([]idb.NACPath, error) {
	var entries []nacSchemaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("nac-schema parse: %w", err)
	}

	auxDocs := map[string]*nacAuxDoc{}
	if h.AuxDir != "" {
		if err := loadNACAuxDocs(h.AuxDir, auxDocs); err != nil {
			return nil, fmt.Errorf("nac-schema load aux docs: %w", err)
		}
	}

	var paths []idb.NACPath
	for _, e := range entries {
		if e.Path == "" {
			continue
		}

		objectName := e.ObjectName
		gui := ""
		description := ""
		examplesJSON := "[]"

		if doc := auxDocs[e.Path]; doc != nil {
			if doc.ObjectName != "" {
				objectName = doc.ObjectName
			}
			gui = doc.GUILocation
			description = doc.Description
			if b, err := json.Marshal(doc.Examples); err == nil {
				examplesJSON = string(b)
			}
		}

		schemaJSON := "{}"
		if len(e.Schema) > 0 {
			schemaJSON = string(e.Schema)
		}

		paths = append(paths, idb.NACPath{
			ProductID:    productID,
			Path:         e.Path,
			ObjectName:   objectName,
			GUILocation:  gui,
			Description:  description,
			Schema:       schemaJSON,
			Examples:     examplesJSON,
			SourceFormat: "nac-schema",
		})
	}

	return paths, nil
}

// loadNACAuxDocs walks dir and loads per-path NAC doc files into out, keyed
// by config path (derived from the filename with the .json suffix stripped).
func loadNACAuxDocs(dir string, out map[string]*nacAuxDoc) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var doc nacAuxDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil // skip malformed files silently
		}

		key := strings.TrimSuffix(filepath.Base(path), ".json")
		out[key] = &doc
		return nil
	})
}
