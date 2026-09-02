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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
	"github.com/cisco-open/cisco-api-guide-mcp/internal/mcp"
	"github.com/cisco-open/cisco-api-guide-mcp/internal/modules"
	"github.com/cisco-open/cisco-api-guide-mcp/internal/search"

	"github.com/urfave/cli/v2"
)

var dbManager *idb.Manager

func main() {
	app := &cli.App{
		Name:  "cisco-api-guide",
		Usage: "Cisco API Guide MCP server",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "http",
				Usage: "Run as a streamable HTTP server instead of stdio",
			},
			&cli.StringFlag{
				Name:  "addr",
				Value: ":8080",
				Usage: "Listen address for HTTP mode (e.g. :8080)",
			},
			&cli.StringFlag{
				Name:    "modules",
				Value:   "all",
				EnvVars: []string{"CISCO_API_MODULES"},
				Usage:   "Comma-separated list of modules to load (e.g. aci,ndfc or all)",
			},
			&cli.StringFlag{
				Name:    "data-dir",
				EnvVars: []string{"CISCO_API_GUIDE_DATA_DIR"},
				Usage:   "Directory to store/cache downloaded module SQLite databases",
			},
			&cli.StringFlag{
				Name:    "registry-url",
				EnvVars: []string{"CISCO_API_REGISTRY_URL"},
				Usage:   "URL to the modules.json registry manifest",
			},
			&cli.BoolFlag{
				Name:  "auto-update",
				Usage: "Check and update cached module DBs if manifest hash changes",
			},
			&cli.BoolFlag{
				Name:  "list-modules",
				Usage: "List locally installed (cached) module databases and exit",
			},
		},
		Action: run,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(c *cli.Context) error {
	dbManager = idb.NewManager()

	fetcher, err := modules.NewModuleFetcher(modules.FetcherOptions{
		DataDir:     c.String("data-dir"),
		RegistryURL: c.String("registry-url"),
	})
	if err != nil {
		return fmt.Errorf("init module fetcher: %w", err)
	}

	if c.Bool("list-modules") {
		return listModules(fetcher)
	}

	rawModules := c.String("modules")
	var requested []string
	if rawModules != "" {
		for _, m := range strings.Split(rawModules, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				requested = append(requested, m)
			}
		}
	}

	paths, err := fetcher.EnsureModules(requested, c.Bool("auto-update"))
	if err != nil {
		return fmt.Errorf("ensure modules: %w", err)
	}

	if err := fetcher.LoadIntoManager(dbManager, paths); err != nil {
		return fmt.Errorf("load modules: %w", err)
	}

	if c.Bool("http") {
		return serveHTTP(c.String("addr"))
	}
	return runStdio()
}

func listModules(fetcher *modules.ModuleFetcher) error {
	infos, err := fetcher.ListLocalModuleInfo()
	if err != nil {
		return fmt.Errorf("list local modules: %w", err)
	}

	if len(infos) == 0 {
		fmt.Printf("No cached modules found in %s\n", fetcher.DataDir())
		return nil
	}

	fmt.Printf("Cached modules in %s:\n\n", fetcher.DataDir())
	for _, info := range infos {
		fmt.Printf("  %-12s %8.1f MB   %s\n", info.Key, float64(info.SizeBytes)/(1024*1024), info.ModTime.Format("2006-01-02 15:04:05"))

		releases, err := loadProductReleases(info.Path)
		if err != nil {
			fmt.Printf("                  (could not read versions: %v)\n", err)
			continue
		}
		if len(releases) == 0 {
			fmt.Printf("                  (no endpoints indexed)\n")
			continue
		}
		for _, r := range releases {
			release := r.Release
			if release == "" {
				release = "(unversioned)"
			}
			fmt.Printf("                  %-10s %-14s %d endpoints\n", r.ProductID, release, r.EndpointCount)
		}
	}
	return nil
}

// loadProductReleases opens a cached module database read-only and returns
// the distinct product/release combinations it contains.
func loadProductReleases(dbPath string) ([]idb.ProductRelease, error) {
	sqlDB, err := idb.OpenFileRO(dbPath)
	if err != nil {
		return nil, err
	}
	defer sqlDB.Close()

	return idb.ListProductReleases(sqlDB)
}

func runStdio() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req mcp.Request
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(writer, mcp.ErrorResponse(nil, -32700, "parse error"))
			continue
		}

		resp, ok := dispatch(req)
		if ok {
			writeResponse(writer, resp)
		}
	}
	return scanner.Err()
}

func writeResponse(w *bufio.Writer, r mcp.Response) {
	b, err := mcp.MarshalResponse(r)
	if err != nil {
		return
	}
	w.Write(b)
	w.Flush()
}

// supportedProtocolVersions lists the MCP protocol versions this server
// understands, newest first. The latest entry is used as the fallback
// when a client doesn't request a version we recognize.
var supportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

func negotiateProtocolVersion(params json.RawMessage) string {
	var req struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &req)
	}
	for _, v := range supportedProtocolVersions {
		if v == req.ProtocolVersion {
			return v
		}
	}
	return supportedProtocolVersions[0]
}

func dispatch(req mcp.Request) (mcp.Response, bool) {
	switch req.Method {
	case "initialize":
		return mcp.OKResponse(req.ID, map[string]interface{}{
			"protocolVersion": negotiateProtocolVersion(req.Params),
			"serverInfo": map[string]interface{}{
				"name":    "cisco-api-guide",
				"version": "0.1.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		}), true

	case "notifications/initialized":
		// Notifications must not produce a response.
		return mcp.Response{}, false

	case "tools/list":
		return mcp.OKResponse(req.ID, map[string]interface{}{
			"tools": mcp.Tools(),
		}), true

	case "tools/call":
		return handleToolCall(req), true

	default:
		return mcp.ErrorResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method)), true
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func handleToolCall(req mcp.Request) mcp.Response {
	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return mcp.ErrorResponse(req.ID, -32602, "invalid params")
	}

	switch p.Name {
	case "search_endpoints":
		return mcp.OKResponse(req.ID, handleSearch(p.Arguments))
	case "get_endpoint":
		return mcp.OKResponse(req.ID, handleGetEndpoint(p.Arguments))
	case "get_product_guide":
		return mcp.OKResponse(req.ID, handleGetProductGuide(p.Arguments))
	case "search_nac_config":
		return mcp.OKResponse(req.ID, handleSearchNACConfig(p.Arguments))
	case "get_nac_config_path":
		return mcp.OKResponse(req.ID, handleGetNACConfigPath(p.Arguments))
	default:
		return mcp.ErrorResponse(req.ID, -32602, fmt.Sprintf("unknown tool: %s", p.Name))
	}
}

func handleSearch(args json.RawMessage) interface{} {
	var a struct {
		Query   string `json:"query"`
		Product string `json:"product"`
		Release string `json:"release"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}
	if a.Query == "" {
		return mcp.ToolErrorResult("query is required")
	}
	if a.Limit == 0 {
		a.Limit = 10
	}

	var productID string
	if a.Product != "" {
		var err error
		productID, err = dbManager.ResolveProduct(a.Product)
		if err != nil {
			return mcp.ToolErrorResult(err.Error())
		}
	}

	ftsQuery := search.BuildFTSQuery(a.Query, dbManager.Synonyms())
	results, total, err := dbManager.SearchEndpoints(ftsQuery, productID, a.Release, a.Limit)
	if err != nil {
		return mcp.ToolErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	if len(results) == 0 {
		return mcp.ToolResult("No results found. Try different keywords or broaden the query.")
	}

	var sb strings.Builder
	for _, r := range results {
		summary := r.Summary
		if summary == "" {
			summary = "(no summary)"
		}
		tag := r.ProductID
		if r.Release != "" {
			tag = r.ProductID + "/" + r.Release
		}
		fmt.Fprintf(&sb, "[%s] %s %s — %s\n", tag, r.Method, r.Path, summary)
	}
	if total > a.Limit {
		fmt.Fprintf(&sb, "\nShowing %d of %d+ results. Use limit parameter for more.", len(results), total-1)
	} else {
		fmt.Fprintf(&sb, "\nShowing %d result(s).", len(results))
	}

	return mcp.ToolResult(sb.String())
}

func handleGetEndpoint(args json.RawMessage) interface{} {
	var a struct {
		Product string `json:"product"`
		Release string `json:"release"`
		Method  string `json:"method"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}

	productID, err := dbManager.ResolveProduct(a.Product)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	e, err := dbManager.GetEndpoint(productID, a.Release, a.Method, a.Path)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	var sb strings.Builder
	header := strings.ToUpper(productID)
	if e.Release != "" {
		header += "/" + e.Release
	}
	fmt.Fprintf(&sb, "%s: %s %s\n", header, e.Method, e.Path)
	if e.Summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n", e.Summary)
	}
	if e.Description != "" {
		fmt.Fprintf(&sb, "\n%s\n", e.Description)
	}

	// Parameters
	var params []map[string]interface{}
	if err := json.Unmarshal([]byte(e.Parameters), &params); err == nil && len(params) > 0 {
		fmt.Fprintf(&sb, "\nParameters:\n")
		for _, p := range params {
			req := ""
			if r, _ := p["required"].(bool); r {
				req = "required"
			} else {
				req = "optional"
			}
			name, _ := p["name"].(string)
			in, _ := p["in"].(string)
			typ, _ := p["type"].(string)
			desc, _ := p["description"].(string)
			fmt.Fprintf(&sb, "  [%s] %s (%s, %s) — %s\n", in, name, req, typ, desc)
			if ex, ok := p["example"].(string); ok && ex != "" {
				fmt.Fprintf(&sb, "    Example: %s\n", ex)
			}
		}
	}

	// Request body
	var rb map[string]interface{}
	if err := json.Unmarshal([]byte(e.RequestBody), &rb); err == nil && len(rb) > 0 {
		fmt.Fprintf(&sb, "\nRequest body:")
		if ct, ok := rb["content_type"].(string); ok {
			fmt.Fprintf(&sb, " (%s)", ct)
		}
		fmt.Fprintf(&sb, "\n")
		if ex, ok := rb["example"]; ok {
			exb, _ := json.MarshalIndent(ex, "  ", "  ")
			fmt.Fprintf(&sb, "  Example: %s\n", string(exb))
		}
	} else {
		fmt.Fprintf(&sb, "\nRequest body: none\n")
	}

	// Responses
	var responses map[string]interface{}
	if err := json.Unmarshal([]byte(e.Responses), &responses); err == nil && len(responses) > 0 {
		fmt.Fprintf(&sb, "\nResponses:\n")
		for status, v := range responses {
			if rv, ok := v.(map[string]interface{}); ok {
				desc, _ := rv["description"].(string)
				fmt.Fprintf(&sb, "  %s — %s\n", status, desc)
			}
		}
	}

	// Tags
	var tags []string
	if err := json.Unmarshal([]byte(e.Tags), &tags); err == nil && len(tags) > 0 {
		fmt.Fprintf(&sb, "\nTags: %s\n", strings.Join(tags, ", "))
	}

	return mcp.ToolResult(sb.String())
}

func handleSearchNACConfig(args json.RawMessage) interface{} {
	var a struct {
		Query   string `json:"query"`
		Product string `json:"product"`
		Release string `json:"release"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}
	if a.Query == "" {
		return mcp.ToolErrorResult("query is required")
	}
	if a.Limit == 0 {
		a.Limit = 10
	}

	var productID string
	if a.Product != "" {
		var err error
		productID, err = dbManager.ResolveProduct(a.Product)
		if err != nil {
			return mcp.ToolErrorResult(err.Error())
		}
	}

	ftsQuery := search.BuildFTSQuery(a.Query, dbManager.Synonyms())
	results, total, err := dbManager.SearchNACPaths(ftsQuery, productID, a.Release, a.Limit)
	if err != nil {
		return mcp.ToolErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	if len(results) == 0 {
		return mcp.ToolResult("No results found. Try different keywords or broaden the query.")
	}

	var sb strings.Builder
	for _, r := range results {
		desc := r.Description
		if desc == "" {
			desc = "(no description)"
		}
		tag := r.ProductID
		if r.Release != "" {
			tag = r.ProductID + "/" + r.Release
		}
		fmt.Fprintf(&sb, "[%s] %s (%s) — %s\n", tag, r.Path, r.ObjectName, desc)
	}
	if total > a.Limit {
		fmt.Fprintf(&sb, "\nShowing %d of %d+ results. Use limit parameter for more.", len(results), total-1)
	} else {
		fmt.Fprintf(&sb, "\nShowing %d result(s).", len(results))
	}

	return mcp.ToolResult(sb.String())
}

func handleGetNACConfigPath(args json.RawMessage) interface{} {
	var a struct {
		Product string `json:"product"`
		Release string `json:"release"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}

	productID, err := dbManager.ResolveProduct(a.Product)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	p, err := dbManager.GetNACPath(productID, a.Release, a.Path)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	var sb strings.Builder
	header := strings.ToUpper(productID)
	if p.Release != "" {
		header += "/" + p.Release
	}
	fmt.Fprintf(&sb, "%s: %s\n", header, p.Path)
	if p.ObjectName != "" {
		fmt.Fprintf(&sb, "Object: %s\n", p.ObjectName)
	}
	if p.GUILocation != "" {
		fmt.Fprintf(&sb, "GUI location: %s\n", p.GUILocation)
	}
	if p.Description != "" {
		fmt.Fprintf(&sb, "\n%s\n", p.Description)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(p.Schema), &schema); err == nil && len(schema) > 0 {
		schemaB, _ := json.MarshalIndent(schema, "  ", "  ")
		fmt.Fprintf(&sb, "\nSchema:\n  %s\n", string(schemaB))
	}

	var examples []map[string]interface{}
	if err := json.Unmarshal([]byte(p.Examples), &examples); err == nil && len(examples) > 0 {
		fmt.Fprintf(&sb, "\nExamples:\n")
		for _, ex := range examples {
			title, _ := ex["title"].(string)
			yaml, _ := ex["yaml"].(string)
			explanation, _ := ex["explanation"].(string)
			if title != "" {
				fmt.Fprintf(&sb, "  %s:\n", title)
			}
			if yaml != "" {
				fmt.Fprintf(&sb, "%s\n", yaml)
			}
			if explanation != "" {
				fmt.Fprintf(&sb, "  %s\n", explanation)
			}
		}
	}

	return mcp.ToolResult(sb.String())
}

func handleGetProductGuide(args json.RawMessage) interface{} {
	var a struct {
		Product string `json:"product"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}

	productID, err := dbManager.ResolveProduct(a.Product)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	p, err := dbManager.GetProduct(productID)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s REST API Guide\n", p.Name)
	if p.BaseURL != "" {
		fmt.Fprintf(&sb, "Base URL: %s\n", p.BaseURL)
	}
	if p.AuthType != "" {
		fmt.Fprintf(&sb, "\nAuthentication: %s\n", p.AuthType)
	}
	if p.AuthNotes != "" {
		fmt.Fprintf(&sb, "%s\n", p.AuthNotes)
	}
	if p.Description != "" {
		fmt.Fprintf(&sb, "\nNotes:\n%s\n", p.Description)
	}

	return mcp.ToolResult(sb.String())
}
