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
	"testing"
)

const validManualJSON = `{
  "endpoints": [
    {
      "method": "GET",
      "path": "/api/v1/items",
      "summary": "List items",
      "description": "Returns all items.",
      "tags": ["items", "core"],
      "parameters": [
        {"name": "limit", "in": "query", "required": false}
      ],
      "request_body": {},
      "responses": {
        "200": {"description": "OK"}
      }
    },
    {
      "method": "POST",
      "path": "/api/v1/items",
      "summary": "Create item",
      "description": "",
      "tags": ["items"],
      "parameters": [],
      "request_body": {
        "content_type": "application/json"
      },
      "responses": {
        "201": {"description": "Created"}
      }
    }
  ]
}`

func TestManualHandler_Parse_BasicFields(t *testing.T) {
	h := &ManualHandler{}
	endpoints, err := h.Parse("ndfc", []byte(validManualJSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}

	first := endpoints[0]
	if first.ProductID != "ndfc" {
		t.Errorf("expected ProductID %q, got %q", "ndfc", first.ProductID)
	}
	if first.Method != "GET" {
		t.Errorf("expected Method %q, got %q", "GET", first.Method)
	}
	if first.Path != "/api/v1/items" {
		t.Errorf("expected Path %q, got %q", "/api/v1/items", first.Path)
	}
	if first.Summary != "List items" {
		t.Errorf("expected Summary %q, got %q", "List items", first.Summary)
	}
	if first.Description != "Returns all items." {
		t.Errorf("expected Description %q, got %q", "Returns all items.", first.Description)
	}
	if first.SourceFormat != "manual" {
		t.Errorf("expected SourceFormat %q, got %q", "manual", first.SourceFormat)
	}
}

func TestManualHandler_Parse_TagsAreValidJSON(t *testing.T) {
	h := &ManualHandler{}
	endpoints, err := h.Parse("ndfc", []byte(validManualJSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	var tags []string
	if err := json.Unmarshal([]byte(endpoints[0].Tags), &tags); err != nil {
		t.Fatalf("Tags is not valid JSON: %v", err)
	}
	if len(tags) != 2 || tags[0] != "items" || tags[1] != "core" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestManualHandler_Parse_ParametersAreValidJSON(t *testing.T) {
	h := &ManualHandler{}
	endpoints, err := h.Parse("ndfc", []byte(validManualJSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	var params []map[string]interface{}
	if err := json.Unmarshal([]byte(endpoints[0].Parameters), &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(params))
	}
	if params[0]["name"] != "limit" {
		t.Errorf("expected param name %q, got %v", "limit", params[0]["name"])
	}
}

func TestManualHandler_Parse_ResponsesAreValidJSON(t *testing.T) {
	h := &ManualHandler{}
	endpoints, err := h.Parse("ndfc", []byte(validManualJSON))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	var responses map[string]interface{}
	if err := json.Unmarshal([]byte(endpoints[0].Responses), &responses); err != nil {
		t.Fatalf("Responses is not valid JSON: %v", err)
	}
	if _, ok := responses["200"]; !ok {
		t.Error("expected 200 key in responses")
	}
}

func TestManualHandler_Parse_NullFieldsDefaultToEmpty(t *testing.T) {
	// Omit optional JSON fields — they should default to empty collections.
	input := `{
		"endpoints": [{
			"method": "DELETE",
			"path": "/api/v1/items/{id}",
			"summary": "Delete item"
		}]
	}`
	h := &ManualHandler{}
	endpoints, err := h.Parse("ndfc", []byte(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	e := endpoints[0]
	if e.Tags != "[]" {
		t.Errorf("expected Tags=%q, got %q", "[]", e.Tags)
	}
	if e.Parameters != "[]" {
		t.Errorf("expected Parameters=%q, got %q", "[]", e.Parameters)
	}
	if e.RequestBody != "{}" {
		t.Errorf("expected RequestBody=%q, got %q", "{}", e.RequestBody)
	}
	if e.Responses != "{}" {
		t.Errorf("expected Responses=%q, got %q", "{}", e.Responses)
	}
}

func TestManualHandler_Parse_EmptyEndpointsList(t *testing.T) {
	input := `{"endpoints": []}`
	h := &ManualHandler{}
	endpoints, err := h.Parse("ndfc", []byte(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(endpoints))
	}
}

func TestManualHandler_Parse_InvalidJSON(t *testing.T) {
	h := &ManualHandler{}
	_, err := h.Parse("ndfc", []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
