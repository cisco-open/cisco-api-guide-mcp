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
