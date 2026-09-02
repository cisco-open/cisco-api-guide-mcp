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
	"regexp"
	"strings"

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
)

// ACIMetaHandler ingests ACI managed-object class metadata.
//
// --input   : path to aci-meta.json (pyaci format)
// --aux-dir : optional path to a directory of per-class APIC JSON doc files
//             (e.g. downloaded from https://<apic>/doc/jsonmeta/<pkg>/<Class>.json)
//
// Without --aux-dir only /api/class/<className>.json GET endpoints are emitted
// (no descriptions, no property details). With --aux-dir the handler also emits
// MO-path endpoints with full descriptions and property schemas.
type ACIMetaHandler struct {
	AuxDir string // optional; set by main.go before Parse is called
}

// ---- pyaci aci-meta.json structures ----------------------------------------

type aciMetaFile struct {
	Classes map[string]aciMetaClass `json:"classes"`
}

type aciMetaClass struct {
	Contains       map[string]string       `json:"contains"`
	IdentifiedBy   []string                `json:"identifiedBy"`
	IsAbstract     bool                    `json:"isAbstract"`
	IsConfigurable bool                    `json:"isConfigurable"`
	IsContextRoot  bool                    `json:"isContextRoot"`
	Properties     map[string]aciMetaProp  `json:"properties"`
	RnFormat       string                  `json:"rnFormat"`
	RnMap          map[string]string       `json:"rnMap"`
}

type aciMetaProp struct {
	IsConfigurable bool `json:"isConfigurable"`
}

// ---- APIC /doc/jsonmeta per-class JSON structures --------------------------

// aciClassDocEntry represents the value under the "pkg:ClassName" top-level key
// in an APIC per-class JSON file.
type aciClassDocEntry struct {
	Label      string                  `json:"label"`
	Comment    []string                `json:"comment"`
	DnFormats  []string                `json:"dnFormats"`
	Properties map[string]aciDocProp   `json:"properties"`
}

type aciDocProp struct {
	Comment     []string       `json:"comment"`
	Label       string         `json:"label"`
	BaseType    string         `json:"baseType"`
	ModelType   string         `json:"modelType"`
	ValidValues []aciValidValue `json:"validValues"`
	ReadOnly    bool           `json:"readOnly"`
	Mandatory   bool           `json:"mandatory"`
	IsNaming    bool           `json:"isNaming"`
	IsHidden    bool           `json:"isHidden"`
}

type aciValidValue struct {
	Value     string `json:"value"`
	LocalName string `json:"localName"`
	Label     string `json:"label"`
}

// ---- class name helpers ----------------------------------------------------

var aciClassRe = regexp.MustCompile(`^([a-z][a-z0-9]*)([A-Z].*)$`)

// splitClassName splits a camelCase ACI class name into package and class portions.
//
//	"fvTenant"    -> ("fv",    "Tenant",   true)
//	"l3extOut"    -> ("l3ext", "Out",      true)
//	"aaaAProvider"-> ("aaa",   "AProvider",true)
func splitClassName(name string) (pkg, cls string, ok bool) {
	m := aciClassRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// maxMOPathsPerClass is the maximum number of MO-path endpoint sets (GET/POST/DELETE)
// generated per ACI class. Some classes (e.g. tag:Annotation, tag:Tag) have tens of
// thousands of DN formats — one for every possible parent object in the fabric — but
// they are semantically identical operations. Storing all of them would inflate the DB
// by orders of magnitude without adding meaningful search value.
//
// Set to 5 to keep a representative sample of DN patterns while keeping DB size viable.
const maxMOPathsPerClass = 5

// maxDNFormatsInDescription caps how many DN format examples are embedded in the class
// endpoint's description field (for user reference and FTS coverage). Listing all
// formats for high-cardinality classes would make individual rows impractically large.
const maxDNFormatsInDescription = 20

// ---- Parse -----------------------------------------------------------------

func (h *ACIMetaHandler) Parse(productID string, data []byte) ([]idb.Endpoint, error) {
	var meta aciMetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("aci-meta parse: %w", err)
	}

	// Load per-class APIC docs from AuxDir (keyed by camelCase class name).
	classDocs := map[string]*aciClassDocEntry{}
	if h.AuxDir != "" {
		if err := loadACIAuxDocs(h.AuxDir, classDocs); err != nil {
			return nil, fmt.Errorf("aci-meta load aux docs: %w", err)
		}
	}

	var endpoints []idb.Endpoint
	for className, cls := range meta.Classes {
		if cls.IsAbstract || !cls.IsConfigurable {
			continue
		}

		pkg, _, ok := splitClassName(className)
		if !ok {
			continue
		}

		doc := classDocs[className]

		// ---- derive display name, description, DN formats ------------------
		summary := className
		description := ""
		var dnFormats []string

		if doc != nil {
			if doc.Label != "" {
				summary = doc.Label
			}
			if len(doc.Comment) > 0 {
				description = strings.Join(doc.Comment, "\n")
			}
			// Filter out empty/whitespace-only dnFormats
			for _, dn := range doc.DnFormats {
				if strings.TrimSpace(dn) != "" {
					dnFormats = append(dnFormats, dn)
				}
			}
		}

		tagsJSON, _ := json.Marshal([]string{pkg})
		tags := string(tagsJSON)
		paramsJSON := buildACIRequestBody(className, cls, doc)

		// ---- build class description with DN format examples ---------------
		// Embed a representative sample of DN formats into the description so
		// they are searchable via FTS and visible to users via get_endpoint.
		classDescription := buildACIClassDescription(description, dnFormats)

		// ---- /api/class/<className>.json  (GET – list all instances) -------
		endpoints = append(endpoints, idb.Endpoint{
			ProductID:    productID,
			Method:       "GET",
			Path:         "/api/class/" + className + ".json",
			Summary:      "List all " + summary + " objects",
			Description:  classDescription,
			Tags:         tags,
			Parameters:   "[]",
			RequestBody:  "{}",
			Responses:    "{}",
			SourceFormat: "aci-meta",
		})

		// ---- /api/mo/<dn>.json  (GET / POST / DELETE) ----------------------
		// Without per-class docs, dnFormats is empty so we skip MO-path entries.
		// With per-class docs dnFormats carries the actual DN patterns.
		//
		// Cap at maxMOPathsPerClass to prevent DB explosion from high-cardinality
		// classes. All DN formats are still referenced in the class GET description.
		moPaths := dnFormats
		if len(moPaths) > maxMOPathsPerClass {
			moPaths = moPaths[:maxMOPathsPerClass]
		}

		for _, dn := range moPaths {
			moPath := "/api/mo/" + dn + ".json"

			endpoints = append(endpoints, idb.Endpoint{
				ProductID:    productID,
				Method:       "GET",
				Path:         moPath,
				Summary:      "Get " + summary + " by distinguished name",
				Description:  description,
				Tags:         tags,
				Parameters:   "[]",
				RequestBody:  "{}",
				Responses:    "{}",
				SourceFormat: "aci-meta",
			})

			endpoints = append(endpoints, idb.Endpoint{
				ProductID:    productID,
				Method:       "POST",
				Path:         moPath,
				Summary:      "Create or update " + summary,
				Description:  description,
				Tags:         tags,
				Parameters:   "[]",
				RequestBody:  paramsJSON,
				Responses:    "{}",
				SourceFormat: "aci-meta",
			})

			endpoints = append(endpoints, idb.Endpoint{
				ProductID:    productID,
				Method:       "DELETE",
				Path:         moPath,
				Summary:      "Delete " + summary + " by distinguished name",
				Description:  description,
				Tags:         tags,
				Parameters:   "[]",
				RequestBody:  "{}",
				Responses:    "{}",
				SourceFormat: "aci-meta",
			})
		}
	}

	return endpoints, nil
}

// buildACIClassDescription appends a representative list of DN format examples
// to the class description so they are searchable via FTS and visible to users.
func buildACIClassDescription(description string, dnFormats []string) string {
	if len(dnFormats) == 0 {
		return description
	}

	var sb strings.Builder
	sb.WriteString(description)
	if description != "" {
		sb.WriteString("\n\n")
	}
	sb.WriteString("DN formats:")

	shown := dnFormats
	extra := 0
	if len(dnFormats) > maxDNFormatsInDescription {
		extra = len(dnFormats) - maxDNFormatsInDescription
		shown = dnFormats[:maxDNFormatsInDescription]
	}
	for _, dn := range shown {
		sb.WriteString("\n  /api/mo/" + dn + ".json")
	}
	if extra > 0 {
		sb.WriteString(fmt.Sprintf("\n  ...and %d more", extra))
	}

	return sb.String()
}

// loadACIAuxDocs walks dir and loads per-class APIC JSON doc files into out.
//
// Each file is expected to contain a single top-level key in the form
// "pkg:ClassName" (e.g. "fv:Tenant") whose value is an aciClassDocEntry.
// The out map is keyed by the camelCase class name (e.g. "fvTenant").
func loadACIAuxDocs(dir string, out map[string]*aciClassDocEntry) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil // skip malformed files silently
		}

		for colonName, docRaw := range raw {
			// "fv:Tenant" -> "fvTenant"
			parts := strings.SplitN(colonName, ":", 2)
			if len(parts) != 2 {
				continue
			}
			camelName := parts[0] + parts[1]

			var entry aciClassDocEntry
			if err := json.Unmarshal(docRaw, &entry); err != nil {
				continue
			}
			out[camelName] = &entry
		}
		return nil
	})
}

// ---- request body helpers --------------------------------------------------

type aciPropSchema struct {
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Type        string   `json:"type,omitempty"`
	Comment     string   `json:"comment,omitempty"`
	Mandatory   bool     `json:"mandatory,omitempty"`
	IsNaming    bool     `json:"isNaming,omitempty"`
	ValidValues []string `json:"validValues,omitempty"`
}

type aciRequestBody struct {
	ClassName  string          `json:"className"`
	Attributes []aciPropSchema `json:"attributes"`
}

// buildACIRequestBody constructs a JSON object describing the class's
// configurable properties, used as the requestBody for POST endpoints.
func buildACIRequestBody(className string, cls aciMetaClass, doc *aciClassDocEntry) string {
	var props []aciPropSchema
	for propName, metaProp := range cls.Properties {
		if !metaProp.IsConfigurable {
			continue
		}
		entry := aciPropSchema{Name: propName}

		if doc != nil {
			if dp, ok := doc.Properties[propName]; ok {
				if dp.IsHidden {
					continue
				}
				entry.Label = dp.Label
				entry.Type = dp.ModelType
				entry.Mandatory = dp.Mandatory
				entry.IsNaming = dp.IsNaming
				if len(dp.Comment) > 0 {
					entry.Comment = strings.Join(dp.Comment, " ")
				}
				for _, vv := range dp.ValidValues {
					entry.ValidValues = append(entry.ValidValues, vv.LocalName)
				}
			}
		}
		props = append(props, entry)
	}

	body := aciRequestBody{ClassName: className, Attributes: props}
	b, _ := json.Marshal(body)
	return string(b)
}
