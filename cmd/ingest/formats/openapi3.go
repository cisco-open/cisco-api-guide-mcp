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
	"strings"

	idb "github.com/brightpuddle/cisco-api-guide-mcp/internal/db"
	"gopkg.in/yaml.v3"
)

// OpenAPI3Handler parses OpenAPI 3.x documents in JSON or YAML format.
type OpenAPI3Handler struct{}

// openAPI3Raw uses interface{} for paths so both JSON and YAML decode cleanly.
type openAPI3Raw struct {
	OpenAPI string `yaml:"openapi" json:"openapi"`
	Info    struct {
		Title   string `yaml:"title"   json:"title"`
		Version string `yaml:"version" json:"version"`
	} `yaml:"info" json:"info"`
	// Paths: path -> (method|field) -> raw operation value
	Paths map[string]map[string]interface{} `yaml:"paths" json:"paths"`
}

// openAPI3Operation is the subset of an OpenAPI operation we store.
type openAPI3Operation struct {
	Summary     string                   `json:"summary"`
	Description string                   `json:"description"`
	Tags        []string                 `json:"tags"`
	OperationID string                   `json:"operationId"`
	Parameters  []interface{}            `json:"parameters"`
	RequestBody map[string]interface{}   `json:"requestBody"`
	Responses   map[string]interface{}   `json:"responses"`
}

var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true,
	"patch": true, "delete": true, "head": true, "options": true,
}

// Parse converts a raw OpenAPI 3.x document (JSON or YAML) into Endpoints.
func (h *OpenAPI3Handler) Parse(productID string, data []byte) ([]idb.Endpoint, error) {
	doc, err := parseOpenAPI3(data)
	if err != nil {
		return nil, fmt.Errorf("openapi3 parse: %w", err)
	}

	var endpoints []idb.Endpoint
	for path, pathItem := range doc.Paths {
		for rawMethod, rawOp := range pathItem {
			method := strings.ToLower(rawMethod)
			if !httpMethods[method] {
				continue // skip x-*, summary, parameters at path level, etc.
			}

			op, err := toOperation(rawOp)
			if err != nil {
				return nil, fmt.Errorf("decode op %s %s: %w", method, path, err)
			}

			endpoints = append(endpoints, idb.Endpoint{
				ProductID:    productID,
				Method:       strings.ToUpper(method),
				Path:         path,
				Summary:      op.Summary,
				Description:  op.Description,
				Tags:         jsonArray(op.Tags),
				Parameters:   jsonArray(op.Parameters),
				RequestBody:  jsonObject(op.RequestBody),
				Responses:    jsonObject(op.Responses),
				SourceFormat: "openapi3",
			})
		}
	}
	return endpoints, nil
}

// parseOpenAPI3 tries JSON first, then YAML.
func parseOpenAPI3(data []byte) (*openAPI3Raw, error) {
	var doc openAPI3Raw
	if err := json.Unmarshal(data, &doc); err == nil {
		return &doc, nil
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("not valid JSON or YAML OpenAPI 3: %w", err)
	}
	return &doc, nil
}

// toOperation round-trips through JSON to normalise the interface{} value from
// either a JSON or YAML decode into an openAPI3Operation struct.
func toOperation(v interface{}) (*openAPI3Operation, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var op openAPI3Operation
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

// jsonArray marshals a slice to a JSON array string, defaulting to "[]".
func jsonArray(v interface{}) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	s := string(b)
	if s == "null" || s == "" {
		return "[]"
	}
	return s
}

// jsonObject marshals a map to a JSON object string, defaulting to "{}".
func jsonObject(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, _ := json.Marshal(v)
	s := string(b)
	if s == "null" || s == "" {
		return "{}"
	}
	return s
}
