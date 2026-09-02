# AGENTS.md

## Project

MCP (Model Context Protocol) server providing LLM-accessible Cisco API documentation for ACI, NDFC, and Intersight (and pluggable modular APIs). Runs over stdio as a JSON-RPC 2.0 server (or streamable HTTP). API documentation modules are downloaded on demand and cached locally.

## Layout

```
cmd/cisco-api-guide/   # Production MCP server binary
cmd/ingest/            # Dev-only DB ingestion CLI
internal/db/           # SQLite queries, schema, types, multi-module DB manager
internal/modules/      # Module fetcher, caching, checksum verification, registry manifest
internal/mcp/          # MCP tool definitions and server
internal/search/       # FTS5 query builder + synonym expansion
scripts/               # Ingestion orchestration (ingest_all.sh)
modules.json           # Registry manifest listing modular API download URLs and SHA256
```

## Tech Stack

- **Go** (CGO-free, static binaries via `ncruces/go-sqlite3` WASM)
- **SQLite** with FTS5 virtual tables and BM25 ranking
- **No web framework** — raw `bufio`/`encoding/json` over stdio or lightweight HTTP
- **Modular DBs** — Downloaded and cached per module in `~/.cache/cisco-api-guide/`

## Build & Test

```sh
go test ./...                          # run tests
go build -o bin/cisco-api-guide ./cmd/cisco-api-guide
```

## Exposed MCP Tools

| Tool | Purpose |
|------|---------|
| `search_endpoints` | BM25 keyword/NL search with synonym expansion |
| `get_endpoint` | Full schema, params, response for a specific method+path |
| `get_product_guide` | Auth workflows per product (ACI cookie, NDFC token, Intersight API key) |

## CLI Options & Modules

```sh
cisco-api-guide --modules aci,ndfc      # load only specific modules
cisco-api-guide --modules all           # load all available modules (default)
cisco-api-guide --data-dir /custom/path # custom SQLite cache directory
cisco-api-guide --auto-update           # check manifest and update cached DBs
```

## DB Generation (dev only)

```sh
ASSETS_DIR=../assets OUTPUT_DIR=./data ./scripts/ingest_all.sh
```

Generates per-product compressed SQLite DBs (`aci.db.gz`, `ndfc.db.gz`, `intersight.db.gz`) and updates `modules.json`.

## Key Patterns

- **FTS5 + synonyms**: Queries expand terms via `synonyms` table (e.g. `vrf` → `virtual routing forwarding vrf-context`) before hitting `endpoints_fts`.
- **Multi-module DB routing**: `db.Manager` aggregates multiple SQLite DBs, executing parallel/unified FTS queries and routing detail calls.
- **Token-efficient output**: Tools return formatted plain text, not JSON, to minimize LLM context usage.
- **Product aliases**: `ucs`/`dcnm` resolve to `intersight`/`ndfc` internally.

## Products

| Slug | Full Name | Ingestion Source |
|------|-----------|-----------------|
| `aci` | Application Centric Infrastructure | APIC JSON metadata + class JSON |
| `ndfc` | Nexus Dashboard Fabric Controller | OpenAPI 3 JSON |
| `intersight` | Cisco Intersight / UCS | OpenAPI 3 YAML |
