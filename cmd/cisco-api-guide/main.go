package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	idb "github.com/brightpuddle/cisco-api-guide-mcp/internal/db"
	"github.com/brightpuddle/cisco-api-guide-mcp/internal/embeddb"
	"github.com/brightpuddle/cisco-api-guide-mcp/internal/mcp"
	"github.com/brightpuddle/cisco-api-guide-mcp/internal/search"
)

var db *sql.DB
var synonyms map[string]string

func main() {
	var err error
	db, err = idb.Open(embeddb.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}

	synonyms, err = idb.GetSynonyms(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load synonyms: %v\n", err)
		os.Exit(1)
	}

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

		resp := dispatch(req)
		writeResponse(writer, resp)
	}
}

func writeResponse(w *bufio.Writer, r mcp.Response) {
	b, err := mcp.MarshalResponse(r)
	if err != nil {
		return
	}
	w.Write(b)
	w.Flush()
}

func dispatch(req mcp.Request) mcp.Response {
	switch req.Method {
	case "initialize":
		return mcp.OKResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]interface{}{
				"name":    "cisco-api-guide",
				"version": "0.1.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		})

	case "notifications/initialized":
		// No response needed
		return mcp.Response{}

	case "tools/list":
		return mcp.OKResponse(req.ID, map[string]interface{}{
			"tools": mcp.Tools(),
		})

	case "tools/call":
		return handleToolCall(req)

	default:
		return mcp.ErrorResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
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
	default:
		return mcp.ErrorResponse(req.ID, -32602, fmt.Sprintf("unknown tool: %s", p.Name))
	}
}

func handleSearch(args json.RawMessage) interface{} {
	var a struct {
		Query   string `json:"query"`
		Product string `json:"product"`
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
		productID, err = idb.ResolveProduct(db, a.Product)
		if err != nil {
			return mcp.ToolErrorResult(err.Error())
		}
	}

	ftsQuery := search.BuildFTSQuery(a.Query, synonyms)
	results, total, err := idb.SearchEndpoints(db, ftsQuery, productID, a.Limit)
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
		fmt.Fprintf(&sb, "[%s] %s %s — %s\n", r.ProductID, r.Method, r.Path, summary)
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
		Method  string `json:"method"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}

	productID, err := idb.ResolveProduct(db, a.Product)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	e, err := idb.GetEndpoint(db, productID, a.Method, a.Path)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %s %s\n", strings.ToUpper(productID), e.Method, e.Path)
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

func handleGetProductGuide(args json.RawMessage) interface{} {
	var a struct {
		Product string `json:"product"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolErrorResult("invalid arguments")
	}

	productID, err := idb.ResolveProduct(db, a.Product)
	if err != nil {
		return mcp.ToolErrorResult(err.Error())
	}

	p, err := idb.GetProduct(db, productID)
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
