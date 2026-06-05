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
	"strings"
	"testing"
)

const minimalOpenAPI3JSON = `{
  "openapi": "3.0.0",
  "info": {"title": "Test API", "version": "1.0"},
  "paths": {
    "/api/v1/resources": {
      "get": {
        "summary": "List resources",
        "description": "Returns all resources.",
        "tags": ["resources"],
        "parameters": [
          {
            "name": "filter",
            "in": "query",
            "required": false,
            "schema": {"type": "string"}
          }
        ],
        "responses": {
          "200": {"description": "Success"}
        }
      },
      "post": {
        "summary": "Create resource",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {"type": "object"}
            }
          }
        },
        "responses": {
          "201": {"description": "Created"},
          "400": {"description": "Bad Request"}
        }
      }
    },
    "/api/v1/resources/{id}": {
      "get": {
        "summary": "Get resource by ID",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK"},
          "404": {"description": "Not Found"}
        }
      },
      "delete": {
        "summary": "Delete resource",
        "responses": {
          "204": {"description": "No Content"}
        }
      }
    }
  }
}`

const minimalOpenAPI3YAML = `
openapi: "3.0.0"
info:
  title: "Test API"
  version: "1.0"
paths:
  /api/v1/things:
    get:
      summary: "List things"
      tags:
        - things
      responses:
        "200":
          description: "OK"
    post:
      summary: "Create thing"
      responses:
        "201":
          description: "Created"
`

func TestOpenAPI3Handler_Parse_JSON(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(minimalOpenAPI3JSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Expect 4 endpoints: GET+POST for /resources, GET+DELETE for /resources/{id}.
	if len(endpoints) != 4 {
		t.Errorf("expected 4 endpoints, got %d", len(endpoints))
	}

	// All endpoints should carry the product ID.
	for _, e := range endpoints {
		if e.ProductID != "ndfc" {
			t.Errorf("expected ProductID %q, got %q", "ndfc", e.ProductID)
		}
		if e.SourceFormat != "openapi3" {
			t.Errorf("expected SourceFormat %q, got %q", "openapi3", e.SourceFormat)
		}
	}
}

func TestOpenAPI3Handler_Parse_MethodsNormalisedToUppercase(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("aci", []byte(minimalOpenAPI3JSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	for _, e := range endpoints {
		if e.Method != strings.ToUpper(e.Method) {
			t.Errorf("method %q is not uppercase", e.Method)
		}
	}
}

func TestOpenAPI3Handler_Parse_SummaryAndDescription(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(minimalOpenAPI3JSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	var found bool
	for _, e := range endpoints {
		if e.Path == "/api/v1/resources" && e.Method == "GET" {
			found = true
			if e.Summary != "List resources" {
				t.Errorf("expected summary %q, got %q", "List resources", e.Summary)
			}
			if e.Description != "Returns all resources." {
				t.Errorf("expected description %q, got %q", "Returns all resources.", e.Description)
			}
		}
	}
	if !found {
		t.Error("GET /api/v1/resources not found in parsed endpoints")
	}
}

func TestOpenAPI3Handler_Parse_ParametersStoredAsJSON(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(minimalOpenAPI3JSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, e := range endpoints {
		if e.Path == "/api/v1/resources/{id}" && e.Method == "GET" {
			var params []map[string]interface{}
			if err := json.Unmarshal([]byte(e.Parameters), &params); err != nil {
				t.Fatalf("Parameters is not valid JSON: %v", err)
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 parameter, got %d", len(params))
			}
			if params[0]["name"] != "id" {
				t.Errorf("expected param name %q, got %v", "id", params[0]["name"])
			}
			if params[0]["in"] != "path" {
				t.Errorf("expected param in=path, got %v", params[0]["in"])
			}
		}
	}
}

func TestOpenAPI3Handler_Parse_ResponsesStoredAsJSON(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(minimalOpenAPI3JSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, e := range endpoints {
		if e.Path == "/api/v1/resources" && e.Method == "POST" {
			var responses map[string]interface{}
			if err := json.Unmarshal([]byte(e.Responses), &responses); err != nil {
				t.Fatalf("Responses is not valid JSON: %v", err)
			}
			if _, has201 := responses["201"]; !has201 {
				t.Error("expected 201 response in POST /api/v1/resources")
			}
			if _, has400 := responses["400"]; !has400 {
				t.Error("expected 400 response in POST /api/v1/resources")
			}
		}
	}
}

func TestOpenAPI3Handler_Parse_TagsStoredAsJSON(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(minimalOpenAPI3JSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, e := range endpoints {
		if e.Path == "/api/v1/resources" && e.Method == "GET" {
			var tags []string
			if err := json.Unmarshal([]byte(e.Tags), &tags); err != nil {
				t.Fatalf("Tags is not valid JSON: %v", err)
			}
			if len(tags) != 1 || tags[0] != "resources" {
				t.Errorf("expected tags [\"resources\"], got %v", tags)
			}
		}
	}
}

func TestOpenAPI3Handler_Parse_YAML(t *testing.T) {
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(minimalOpenAPI3YAML))
	if err != nil {
		t.Fatalf("Parse returned error for YAML: %v", err)
	}
	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints from YAML, got %d", len(endpoints))
	}
	found := false
	for _, e := range endpoints {
		if e.Path == "/api/v1/things" && e.Method == "GET" {
			found = true
			if e.Summary != "List things" {
				t.Errorf("expected summary %q, got %q", "List things", e.Summary)
			}
		}
	}
	if !found {
		t.Error("GET /api/v1/things not found in YAML-parsed endpoints")
	}
}

func TestOpenAPI3Handler_Parse_SkipsNonHTTPMethods(t *testing.T) {
	// Path-level fields like "summary" and "parameters" should not be treated
	// as HTTP method operations.
	doc := `{
		"openapi": "3.0.0",
		"info": {"title": "T", "version": "1"},
		"paths": {
			"/api/v1/items": {
				"summary": "Items path",
				"parameters": [],
				"get": {
					"summary": "List items",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`
	h := &OpenAPI3Handler{}
	endpoints, err := h.Parse("ndfc", []byte(doc))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint (only GET), got %d", len(endpoints))
		for _, e := range endpoints {
			t.Logf("  %s %s", e.Method, e.Path)
		}
	}
}

func TestOpenAPI3Handler_Parse_InvalidInput(t *testing.T) {
	h := &OpenAPI3Handler{}
	_, err := h.Parse("ndfc", []byte("this is not json or yaml at all {{{"))
	if err == nil {
		t.Error("expected error for invalid input, got nil")
	}
}

// ---- jsonArray / jsonObject helpers ----------------------------------------

func TestJSONArray_NilInput(t *testing.T) {
	got := jsonArray(nil)
	if got != "[]" {
		t.Errorf("expected %q, got %q", "[]", got)
	}
}

func TestJSONArray_NonNilSlice(t *testing.T) {
	got := jsonArray([]string{"a", "b"})
	var parsed []string
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("jsonArray output is not valid JSON: %v", err)
	}
	if len(parsed) != 2 || parsed[0] != "a" || parsed[1] != "b" {
		t.Errorf("unexpected parsed value: %v", parsed)
	}
}

func TestJSONObject_NilInput(t *testing.T) {
	got := jsonObject(nil)
	if got != "{}" {
		t.Errorf("expected %q, got %q", "{}", got)
	}
}

func TestJSONObject_NonNilMap(t *testing.T) {
	got := jsonObject(map[string]string{"key": "value"})
	var parsed map[string]string
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("jsonObject output is not valid JSON: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("expected key=value, got %v", parsed)
	}
}
