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

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// serveHTTP starts a streamable HTTP MCP server on the given address.
// The server exposes the same three tools as the stdio mode and serves
// them at the /mcp endpoint, which is the path required by Circuit.
func serveHTTP(addr string) error {
	s := server.NewMCPServer(
		"cisco-api-guide",
		"0.1.0",
		server.WithRecovery(),
	)

	s.AddTool(searchEndpointsTool(), searchEndpointsHandler)
	s.AddTool(getEndpointTool(), getEndpointHandler)
	s.AddTool(getProductGuideTool(), getProductGuideHandler)
	s.AddTool(searchNACConfigTool(), searchNACConfigHandler)
	s.AddTool(getNACConfigPathTool(), getNACConfigPathHandler)

	hs := server.NewStreamableHTTPServer(s,
		server.WithEndpointPath("/mcp"),
	)

	fmt.Printf("cisco-api-guide MCP server listening on %s/mcp\n", addr)
	return hs.Start(addr)
}

// --- Tool definitions ---

func searchEndpointsTool() mcp.Tool {
	return mcp.NewTool("search_endpoints",
		mcp.WithDescription("Search Cisco API endpoints by natural language or keywords. Returns ranked list of matching endpoints."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Natural language or keyword search. Example: 'query VRFs in a tenant' or 'create EPG'"),
		),
		mcp.WithString("product",
			mcp.Description("Filter to a specific product. One of: aci, ndfc, intersight (aliases: ucs, dcnm). Omit to search all products."),
			mcp.Enum("aci", "ndfc", "intersight", "ucs", "dcnm"),
		),
		mcp.WithString("release",
			mcp.Description("Filter by release using prefix matching. Example: '3' matches '3.2.2m'. Omit to search all releases."),
		),
		mcp.WithInteger("limit",
			mcp.Description("Max results to return. Default: 10. Max: 50."),
			mcp.DefaultNumber(10),
		),
	)
}

func getEndpointTool() mcp.Tool {
	return mcp.NewTool("get_endpoint",
		mcp.WithDescription("Get full detail for a specific Cisco API endpoint including parameters, request body, and responses."),
		mcp.WithString("product",
			mcp.Required(),
			mcp.Description("Product slug. One of: aci, ndfc, intersight (aliases: ucs, dcnm)."),
			mcp.Enum("aci", "ndfc", "intersight", "ucs", "dcnm"),
		),
		mcp.WithString("method",
			mcp.Required(),
			mcp.Description("HTTP method."),
			mcp.Enum("GET", "POST", "PUT", "DELETE", "PATCH"),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("API path as returned by search_endpoints. Example: /api/node/class/{class}.json"),
		),
		mcp.WithString("release",
			mcp.Description("Release prefix to select a specific version (e.g. '3' or '3.2.2m'). Required when multiple releases exist for the same path."),
		),
	)
}

func getProductGuideTool() mcp.Tool {
	return mcp.NewTool("get_product_guide",
		mcp.WithDescription("Get authentication instructions and general usage notes for a Cisco product API."),
		mcp.WithString("product",
			mcp.Required(),
			mcp.Description("Product slug. One of: aci, ndfc, intersight (aliases: ucs, dcnm), nac-aci, nac-vxlan."),
			mcp.Enum("aci", "ndfc", "intersight", "ucs", "dcnm", "nac-aci", "nac-vxlan"),
		),
	)
}

func searchNACConfigTool() mcp.Tool {
	return mcp.NewTool("search_nac_config",
		mcp.WithDescription("Search Cisco Network-as-Code (NaC) YAML configuration paths by natural language or keywords. Returns ranked list of matching config paths."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Natural language or keyword search. Example: 'static VLAN pool' or 'bootstrap admin user'"),
		),
		mcp.WithString("product",
			mcp.Description("Filter to a specific NaC product. One of: nac-aci, nac-vxlan. Omit to search all NaC products."),
			mcp.Enum("nac-aci", "nac-vxlan"),
		),
		mcp.WithString("release",
			mcp.Description("Filter by release using prefix matching. Example: '2' matches '2.0.0'. Omit to search all releases."),
		),
		mcp.WithInteger("limit",
			mcp.Description("Max results to return. Default: 10. Max: 50."),
			mcp.DefaultNumber(10),
		),
	)
}

func getNACConfigPathTool() mcp.Tool {
	return mcp.NewTool("get_nac_config_path",
		mcp.WithDescription("Get full detail for a specific Cisco Network-as-Code (NaC) YAML configuration path, including schema, GUI location, and worked examples."),
		mcp.WithString("product",
			mcp.Required(),
			mcp.Description("NaC product slug. One of: nac-aci, nac-vxlan."),
			mcp.Enum("nac-aci", "nac-vxlan"),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("NaC config path as returned by search_nac_config. Example: apic.access_policies.vlan_pools"),
		),
		mcp.WithString("release",
			mcp.Description("Release prefix to select a specific version (e.g. '2' or '2.0.0'). Required when multiple releases exist for the same path."),
		),
	)
}

// --- Tool handlers (delegate to existing business logic) ---

func searchEndpointsHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := marshalArgs(req.GetRawArguments())
	return toolResultFromLegacy(handleSearch(raw)), nil
}

func getEndpointHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := marshalArgs(req.GetRawArguments())
	return toolResultFromLegacy(handleGetEndpoint(raw)), nil
}

func getProductGuideHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := marshalArgs(req.GetRawArguments())
	return toolResultFromLegacy(handleGetProductGuide(raw)), nil
}

func searchNACConfigHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := marshalArgs(req.GetRawArguments())
	return toolResultFromLegacy(handleSearchNACConfig(raw)), nil
}

func getNACConfigPathHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := marshalArgs(req.GetRawArguments())
	return toolResultFromLegacy(handleGetNACConfigPath(raw)), nil
}

// marshalArgs converts the mcp-go arguments (map[string]any) back to json.RawMessage
// so we can reuse the existing handler functions unchanged.
func marshalArgs(args any) json.RawMessage {
	b, err := json.Marshal(args)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// toolResultFromLegacy converts the legacy map-based tool result (produced by
// the hand-rolled internal/mcp helpers) into a mcp-go CallToolResult.
// The legacy format is either:
//
//	{"content": [{"type":"text","text":"..."}]}          (success)
//	{"isError": true, "content": [{"type":"text","text":"..."}]}  (error)
func toolResultFromLegacy(v interface{}) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError("internal error marshalling result")
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &result); err != nil || len(result.Content) == 0 {
		return mcp.NewToolResultError("internal error parsing result")
	}

	text := result.Content[0].Text
	if result.IsError {
		return mcp.NewToolResultError(text)
	}
	return mcp.NewToolResultText(text)
}
