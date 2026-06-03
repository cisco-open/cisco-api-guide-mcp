# cisco-api-guide — Implementation Plan

## Goal

Standalone stdio MCP server providing token-efficient, searchable access to Cisco product API
documentation (ACI, NDFC, Intersight/UCS). Distributed as a single static Go binary per
platform. API data ships pre-baked into the binary via embedded SQLite. New API data = new
semantic release.

Target clients: Claude Desktop, Claude Code, opencode, GitHub Copilot, VS Code (MCP
extension), Antigravity, Pi, and any other MCP-compliant agentic harness.

---

## Key Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| MCP transport | stdio only | Universal client compatibility; no auth surface |
| Storage | SQLite via `ncruces/go-sqlite3` | CGO_ENABLED=0 compatible (WASM-based) |
| DB delivery | `//go:embed data/api.db` | Single binary; data versioned with code |
| Search | FTS5 + synonym expansion table | No runtime model dep; covers 90%+ real queries |
| Ingestion | Separate `cmd/ingest` binary (dev-only) | Keeps MCP server CLI clean; not in release |
| Release | goreleaser → GitHub Releases (zip) | linux_amd64, darwin_arm64, windows_amd64 |

---

## Supported Products

| Slug | Name | Aliases |
|---|---|---|
| `aci` | Cisco ACI | — |
| `ndfc` | Cisco NDFC | `dcnm` |
| `intersight` | Cisco Intersight | `ucs` |

---

## Repository Layout

```
cisco-api-guide-mcp/
├── cmd/
│   ├── cisco-api-guide/        # Production MCP server (released binary)
│   │   └── main.go
│   └── ingest/                 # Dev-only ingestion tool (NOT in goreleaser)
│       └── main.go
├── internal/
│   ├── db/
│   │   ├── db.go               # Open embedded DB (copy to temp, open read-only)
│   │   ├── schema.sql          # Canonical DDL (embedded at build, applied by ingest)
│   │   ├── queries.go          # All SQL queries
│   │   └── types.go            # Go structs for DB rows
│   ├── mcp/
│   │   ├── server.go           # MCP server init (fastmcp or manual JSON-RPC)
│   │   └── tools.go            # Tool handler implementations
│   └── search/
│       └── fts.go              # Query builder: synonym expansion + FTS5 query
├── data/
│   └── api.db                  # Pre-built SQLite DB (committed to repo; embedded)
├── .goreleaser.yml
├── go.mod
├── go.sum
├── README.md
└── plan.md
```

---

## Database Schema

Full DDL lives in `internal/db/schema.sql` and is embedded/applied by `cmd/ingest`.
The production binary never runs DDL — it opens the pre-built `data/api.db` read-only.

```sql
-- Products: one row per supported Cisco product
CREATE TABLE IF NOT EXISTS products (
    id           TEXT PRIMARY KEY,   -- 'aci', 'ndfc', 'intersight'
    name         TEXT NOT NULL,      -- Display name: "Cisco ACI"
    description  TEXT NOT NULL DEFAULT '',
    base_url     TEXT NOT NULL DEFAULT '',  -- e.g. https://<apic>/api
    auth_type    TEXT NOT NULL DEFAULT '',  -- 'basic', 'token', 'oauth2', 'apikey'
    auth_notes   TEXT NOT NULL DEFAULT '',  -- Human-readable auth instructions
    auth_schema  TEXT NOT NULL DEFAULT '{}' -- JSON: structured auth parameters
);

-- Aliases: map alternate slugs to canonical product id
CREATE TABLE IF NOT EXISTS product_aliases (
    alias      TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id)
);

-- Endpoints: one row per HTTP operation
CREATE TABLE IF NOT EXISTS endpoints (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    TEXT NOT NULL REFERENCES products(id),
    method        TEXT NOT NULL,          -- GET, POST, PUT, DELETE, PATCH
    path          TEXT NOT NULL,          -- e.g. /api/mo/{dn}.json
    summary       TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    tags          TEXT NOT NULL DEFAULT '[]',     -- JSON array of strings
    parameters    TEXT NOT NULL DEFAULT '[]',     -- JSON array (see Parameters spec below)
    request_body  TEXT NOT NULL DEFAULT '{}',     -- JSON (see RequestBody spec below)
    responses     TEXT NOT NULL DEFAULT '{}',     -- JSON map status→ResponseSpec
    source_format TEXT NOT NULL DEFAULT '',       -- 'openapi3', 'swagger2', 'manual'
    UNIQUE(product_id, method, path)
);

-- FTS5 virtual table for full-text search
-- content= keeps FTS in sync with endpoints table
-- tokenize=porter enables stemming (query → queries)
CREATE VIRTUAL TABLE IF NOT EXISTS endpoints_fts USING fts5(
    summary,
    description,
    path,
    tags,
    content='endpoints',
    content_rowid='id',
    tokenize='porter unicode61'
);

-- Triggers to keep FTS in sync (used by ingest tool)
CREATE TRIGGER IF NOT EXISTS endpoints_fts_insert
    AFTER INSERT ON endpoints BEGIN
    INSERT INTO endpoints_fts(rowid, summary, description, path, tags)
    VALUES (new.id, new.summary, new.description, new.path, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS endpoints_fts_delete
    BEFORE DELETE ON endpoints BEGIN
    INSERT INTO endpoints_fts(endpoints_fts, rowid, summary, description, path, tags)
    VALUES ('delete', old.id, old.summary, old.description, old.path, old.tags);
END;

CREATE TRIGGER IF NOT EXISTS endpoints_fts_update
    AFTER UPDATE ON endpoints BEGIN
    INSERT INTO endpoints_fts(endpoints_fts, rowid, summary, description, path, tags)
    VALUES ('delete', old.id, old.summary, old.description, old.path, old.tags);
    INSERT INTO endpoints_fts(rowid, summary, description, path, tags)
    VALUES (new.id, new.summary, new.description, new.path, new.tags);
END;

-- Synonym expansion: used at query time to broaden FTS queries
CREATE TABLE IF NOT EXISTS synonyms (
    term       TEXT NOT NULL,   -- canonical or variant form
    expansion  TEXT NOT NULL    -- space-separated additional search terms
);

CREATE INDEX IF NOT EXISTS synonyms_term ON synonyms(term);
```

### JSON field specs (for ingest tooling reference)

**`parameters`** — JSON array of parameter objects:
```json
[
  {
    "name": "dn",
    "in": "path",           // "path", "query", "header", "body"
    "required": true,
    "type": "string",       // "string", "integer", "boolean", "object", "array"
    "description": "Distinguished name of the managed object",
    "example": "uni/tn-Production"
  }
]
```

**`request_body`** — JSON object (empty `{}` if none):
```json
{
  "required": true,
  "content_type": "application/json",
  "schema": {},             // JSON Schema fragment or example object
  "example": {}
}
```

**`responses`** — JSON map of status code → response spec:
```json
{
  "200": {
    "description": "Success",
    "content_type": "application/json",
    "schema": {}
  },
  "400": { "description": "Bad request" }
}
```

**`auth_schema`** in products table:
```json
{
  "fields": [
    { "name": "username", "type": "string", "description": "APIC admin user" },
    { "name": "password", "type": "string", "secret": true }
  ],
  "flow": "POST /api/aaaLogin.json → extract token from response → set Cookie: APIC-cookie=<token>"
}
```

### Seed synonyms

Pre-populate on first ingest:

| term | expansion |
|---|---|
| vrf | virtual routing forwarding vrf-context |
| bd | bridge domain |
| epg | endpoint group |
| vlan | vlan-id network segmentation |
| tenant | tn org organization |
| contract | filter subject policy |
| l3out | l3 external routed network |
| fabric | infrastructure underlay |
| vpc | virtual port channel |
| ucs | intersight |
| intersight | ucs server compute |

(Expand this list as real-world queries reveal gaps.)

---

## MCP Tools Specification

Three tools. All accept optional `product` param to scope results.

### Tool 1: `search_endpoints`

Search for API endpoints using natural language or keywords.

**Input schema:**
```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Natural language or keyword search. Example: 'query VRFs in a tenant' or 'create EPG'"
    },
    "product": {
      "type": "string",
      "description": "Filter to a specific product. One of: aci, ndfc, intersight (aliases: ucs, dcnm). Omit to search all products.",
      "enum": ["aci", "ndfc", "intersight", "ucs", "dcnm"]
    },
    "limit": {
      "type": "integer",
      "description": "Max results to return. Default: 10. Max: 50.",
      "default": 10
    }
  },
  "required": ["query"]
}
```

**Output:** Plain text list of results, one per line, ranked by FTS5 BM25 score:
```
[aci] GET /api/node/class/{class}.json — Query managed objects by class (e.g. fvTenant, fvCtx)
[aci] GET /api/mo/{dn}.json — Read a specific managed object by distinguished name
...
Showing 5 of 12 results. Use limit parameter for more.
```

**Implementation:**
1. Resolve product alias → canonical id if `product` provided
2. Expand query terms using synonyms table
3. Build FTS5 query: tokenize input, append expansions, use `OR` for synonyms, wrap in `"..."` for phrases
4. Query: `SELECT e.id, e.product_id, e.method, e.path, e.summary FROM endpoints e JOIN endpoints_fts f ON e.id = f.rowid WHERE endpoints_fts MATCH ? [AND e.product_id = ?] ORDER BY rank LIMIT ?`
5. Format output as plain text (token-efficient, not JSON)

**FTS query construction example:**
- Input: `"query VRFs"`
- After synonym expansion: `vrf OR "virtual routing forwarding" OR "vrf-context"`
- FTS5 query: `query (vrf OR "virtual routing forwarding" OR "vrf-context")`

### Tool 2: `get_endpoint`

Get full detail for a specific API endpoint.

**Input schema:**
```json
{
  "type": "object",
  "properties": {
    "product": {
      "type": "string",
      "description": "Product slug. One of: aci, ndfc, intersight (aliases: ucs, dcnm).",
      "enum": ["aci", "ndfc", "intersight", "ucs", "dcnm"]
    },
    "method": {
      "type": "string",
      "description": "HTTP method. Example: GET, POST, PUT, DELETE",
      "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"]
    },
    "path": {
      "type": "string",
      "description": "API path as returned by search_endpoints. Example: /api/node/class/{class}.json"
    }
  },
  "required": ["product", "method", "path"]
}
```

**Output:** Structured plain text (not JSON — token efficient):
```
ACI: GET /api/node/class/{class}.json
Summary: Query managed objects by class

Parameters:
  [path] class (required, string) — MO class name. Example: fvTenant
  [query] query-target (optional, string) — Filter scope. Example: subtree
  [query] rsp-subtree (optional, string) — Include subtree. Values: no, children, full

Request body: none

Responses:
  200 — JSON object with imdata array containing matched MOs
  400 — Invalid class or query filter

Tags: objects, query, read
```

**Implementation:**
- Resolve alias
- `SELECT * FROM endpoints WHERE product_id = ? AND method = ? AND path = ?`
- Parse JSON fields and render as formatted plain text

### Tool 3: `get_product_guide`

Get authentication instructions and general usage notes for a product's API.

**Input schema:**
```json
{
  "type": "object",
  "properties": {
    "product": {
      "type": "string",
      "description": "Product slug. One of: aci, ndfc, intersight (aliases: ucs, dcnm).",
      "enum": ["aci", "ndfc", "intersight", "ucs", "dcnm"]
    }
  },
  "required": ["product"]
}
```

**Output:** Plain text guide:
```
Cisco ACI REST API Guide
Base URL: https://<apic-host>/api

Authentication: Token-based (APIC cookie)
1. POST /api/aaaLogin.json with body: {"aaaUser":{"attributes":{"name":"admin","pwd":"<password>"}}}
2. Extract token from response: imdata[0].aaaLogin.attributes.token
3. Set cookie on subsequent requests: APIC-cookie=<token>
Token expires after 600 seconds of inactivity (refresh with POST /api/aaaRefresh.json).

Notes:
- All responses wrapped in {"imdata": [...]}
- Distinguished names (DN) identify every object: uni/tn-<name>/bd-<name>
- Append .json or .xml to paths for format selection
```

---

## MCP Server Implementation

### Protocol

Implement MCP protocol manually over stdio (JSON-RPC 2.0). Do NOT use a framework that adds HTTP/SSE — stdio only.

Sequence:
1. Read `initialize` request from stdin → respond with server info + capabilities
2. Read `tools/list` → respond with all three tool definitions
3. Read `tools/call` → dispatch to handler → respond with result

Use `bufio.Scanner` on stdin, one JSON object per line (newline-delimited JSON-RPC).

### DB access at runtime

```go
//go:embed data/api.db
var embeddedDB []byte

func Open() (*sql.DB, error) {
    // Write to temp file (ncruces/go-sqlite3 requires file path)
    tmp, err := os.CreateTemp("", "cisco-api-guide-*.db")
    // write embeddedDB bytes → tmp
    // open as read-only: "file:/path?mode=ro"
    return sql.Open("sqlite3", "file:"+tmp.Name()+"?mode=ro")
}
```

Alternative: use `ncruces/go-sqlite3`'s VFS support to open directly from bytes without temp file — investigate during implementation.

### Error handling

Return MCP error response (not panic) for:
- Unknown product slug
- Empty search results (return helpful message, not error)
- DB errors

---

## Ingestion Tool (`cmd/ingest`)

Dev-only binary. Not included in goreleaser. Run manually by maintainers.

### CLI interface

```
cisco-api-guide-ingest --product aci --format openapi3 --input ./aci-api.json
cisco-api-guide-ingest --product ndfc --format swagger2 --input ./ndfc-swagger.json
cisco-api-guide-ingest --product intersight --format openapi3 --input ./intersight-api.json
cisco-api-guide-ingest --synonyms ./synonyms.csv   # bulk-load synonym table
cisco-api-guide-ingest --product aci --init         # seed product metadata only
```

Flags:
- `--db` — path to SQLite DB file (default: `./data/api.db`)
- `--product` — product slug
- `--format` — input format: `openapi3`, `swagger2`, `manual` (more added over time)
- `--input` — input file path or `-` for stdin
- `--synonyms` — CSV file of `term,expansion` pairs
- `--init` — write product metadata row without ingesting endpoints

### Format handler interface

```go
type FormatHandler interface {
    // Parse returns endpoints extracted from raw input bytes.
    Parse(productID string, data []byte) ([]db.Endpoint, error)
}

var handlers = map[string]FormatHandler{
    "openapi3": &OpenAPI3Handler{},
    "swagger2": &Swagger2Handler{},
    "manual":   &ManualHandler{},  // simple JSON matching internal schema
}
```

Currently, all handlers return `fmt.Errorf("format %q: not implemented")`. Implement one at a time as real API docs are acquired.

### Workflow for adding new API data

1. Obtain API doc (swagger.json, openapi.yaml, etc.) from Cisco dev portal or running instance
2. Run: `cisco-api-guide-ingest --product <slug> --format <format> --input <file> --db ./data/api.db`
3. If format unsupported: implement new `FormatHandler` in `cmd/ingest/formats/`
4. Verify with: `cisco-api-guide search_endpoints "some query"` (test mode, not MCP)
5. Commit updated `data/api.db`
6. Tag new semantic version → goreleaser publishes release

---

## Goreleaser Configuration

File: `.goreleaser.yml`

```yaml
version: 2

before:
  hooks:
    - rm -rf dist
    - go mod tidy
    - go generate ./...

builds:
  - id: cisco-api-guide
    binary: cisco-api-guide
    main: ./cmd/cisco-api-guide
    targets:
      - windows_amd64_v1
      - linux_amd64_v1
      - darwin_arm64
    env:
      - CGO_ENABLED=0
    tags:
      - netgo
      - static_build

release:
  github:
    owner: brightpuddle
    name: cisco-api-guide-mcp

archives:
  - id: cisco-api-guide
    formats: ["zip"]
    files:
      - README.md
      - LICENSE
```

Note: `cmd/ingest` is deliberately excluded from builds list.

---

## Module and Dependencies

```
module github.com/brightpuddle/cisco-api-guide-mcp

go 1.23
```

Dependencies:
- `github.com/ncruces/go-sqlite3` — SQLite driver (CGO_ENABLED=0 via WASM)
- No other runtime dependencies

Dev-only (ingest tool only):
- `github.com/urfave/cli/v2` — CLI flag parsing for ingest tool
- `gopkg.in/yaml.v3` — YAML parsing for OpenAPI inputs

---

## MCP Client Configuration

### Claude Desktop (`claude_desktop_config.json`)
```json
{
  "mcpServers": {
    "cisco-api-guide": {
      "command": "/path/to/cisco-api-guide",
      "args": []
    }
  }
}
```

### Claude Code (`.claude/settings.json`)
```json
{
  "mcpServers": {
    "cisco-api-guide": {
      "command": "cisco-api-guide",
      "type": "stdio"
    }
  }
}
```

### VS Code (`settings.json`)
```json
{
  "mcp.servers": {
    "cisco-api-guide": {
      "command": "cisco-api-guide",
      "type": "stdio"
    }
  }
}
```

README should include configuration snippets for all major clients.

---

## Implementation Phases

### Phase 1 — Skeleton (first session)
- [ ] `go.mod` init, deps
- [ ] `internal/db/` — schema, Open(), stub queries
- [ ] `data/api.db` — empty DB with schema applied
- [ ] `cmd/cisco-api-guide/main.go` — stdio JSON-RPC loop (initialize + tools/list only)
- [ ] Three tool definitions wired (no real query yet — return stub responses)
- [ ] `.goreleaser.yml`
- [ ] README with install + client config snippets

### Phase 2 — Search and retrieval
- [ ] FTS5 queries in `internal/db/queries.go`
- [ ] Synonym expansion in `internal/search/fts.go`
- [ ] `search_endpoints` tool — real DB query + formatted output
- [ ] `get_endpoint` tool — real DB query + formatted output
- [ ] `get_product_guide` tool — real DB query + formatted output
- [ ] Alias resolution

### Phase 3 — Ingestion tool skeleton
- [ ] `cmd/ingest/main.go` — CLI flags, handler dispatch
- [ ] `FormatHandler` interface
- [ ] `manual` format handler (accepts JSON matching internal schema directly)
- [ ] `openapi3` handler — stub returning not-implemented
- [ ] `swagger2` handler — stub returning not-implemented
- [ ] Synonym bulk-load
- [ ] Product `--init` seed command

### Phase 4 — Real data + formats (future sessions, per product)
- [ ] Implement `openapi3` handler
- [ ] Implement `swagger2` handler
- [ ] Ingest ACI API docs → commit `data/api.db` → tag v0.1.0
- [ ] Ingest NDFC API docs → tag v0.2.0
- [ ] Ingest Intersight API docs → tag v0.3.0

### Phase 5 — CAF integration (separate project, future)
- [ ] HTTP transport variant
- [ ] JWT middleware (CircuIT JWKS validation)
- [ ] CAF API endpoints for ingest
- [ ] Re-use same `internal/db` and `internal/search` packages

---

## Open Questions (resolved)

- **FTS vs embeddings**: FTS5 + synonyms. No embeddings — no runtime model for query-time processing.
- **Driver**: `ncruces/go-sqlite3` (CGO_ENABLED=0 compatible).
- **DB delivery**: `//go:embed data/api.db` — data = binary, new data = new release.
- **Binary name**: `cisco-api-guide`
- **Transport**: stdio only
- **Products**: `aci`, `ndfc`, `intersight` (aliases: `ucs`, `dcnm`)
- **Ingestion**: separate `cmd/ingest` binary, dev-only, excluded from releases
