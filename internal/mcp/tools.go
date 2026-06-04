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

package mcp

// ToolDef is the JSON structure for an MCP tool definition.
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// Tools returns all three tool definitions.
func Tools() []ToolDef {
	return []ToolDef{
		{
			Name:        "search_endpoints",
			Description: "Search Cisco API endpoints by natural language or keywords. Returns ranked list of matching endpoints.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Natural language or keyword search. Example: 'query VRFs in a tenant' or 'create EPG'",
					},
					"product": map[string]interface{}{
						"type":        "string",
						"description": "Filter to a specific product. One of: aci, ndfc, intersight (aliases: ucs, dcnm). Omit to search all products.",
						"enum":        []string{"aci", "ndfc", "intersight", "ucs", "dcnm"},
					},
					"release": map[string]interface{}{
						"type":        "string",
						"description": "Filter by release using prefix matching. Example: '3' matches '3.2.2m'. Omit to search all releases.",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Max results to return. Default: 10. Max: 50.",
						"default":     10,
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "get_endpoint",
			Description: "Get full detail for a specific Cisco API endpoint including parameters, request body, and responses.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"product": map[string]interface{}{
						"type":        "string",
						"description": "Product slug. One of: aci, ndfc, intersight (aliases: ucs, dcnm).",
						"enum":        []string{"aci", "ndfc", "intersight", "ucs", "dcnm"},
					},
					"release": map[string]interface{}{
						"type":        "string",
						"description": "Release prefix to select a specific version (e.g. '3' or '3.2.2m'). Required when multiple releases exist for the same path.",
					},
					"method": map[string]interface{}{
						"type":        "string",
						"description": "HTTP method.",
						"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "API path as returned by search_endpoints. Example: /api/node/class/{class}.json",
					},
				},
				"required": []string{"product", "method", "path"},
			},
		},
		{
			Name:        "get_product_guide",
			Description: "Get authentication instructions and general usage notes for a Cisco product API.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"product": map[string]interface{}{
						"type":        "string",
						"description": "Product slug. One of: aci, ndfc, intersight (aliases: ucs, dcnm).",
						"enum":        []string{"aci", "ndfc", "intersight", "ucs", "dcnm"},
					},
				},
				"required": []string{"product"},
			},
		},
	}
}
