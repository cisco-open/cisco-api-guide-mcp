# AGENTS.md

## Project

MCP (Model Context Protocol) server providing LLM-accessible Cisco API documentation for ACI, NDFC, and Intersight. Runs over stdio as a JSON-RPC 2.0 server. Database is embedded in the binary at build time.

## Layout

```
cmd/cisco-api-guide/   # Production MCP server binary
cmd/ingest/            # Dev-only DB ingestion CLI
internal/db/           # SQLite queries, schema, types
internal/embeddb/      # api.db + go:embed wiring
internal/mcp/          # MCP tool definitions and server
internal/search/       # FTS5 query builder + synonym expansion
scripts/               # Ingestion orchestration (ingest_all.sh)
```

## Tech Stack

- **Go** (CGO-free, static binaries via `ncruces/go-sqlite3` WASM)
- **SQLite** with FTS5 virtual tables and BM25 ranking
- **No web framework** — raw `bufio`/`encoding/json` over stdio
- **Embedded DB** — `api.db` baked into binary via `//go:embed`

## Build & Test

```sh
go test ./...                          # run tests
go build -o cisco-api-guide ./cmd/cisco-api-guide
```

## Exposed MCP Tools

| Tool | Purpose |
|------|---------|
| `search_endpoints` | BM25 keyword/NL search with synonym expansion |
| `get_endpoint` | Full schema, params, response for a specific method+path |
| `get_product_guide` | Auth workflows per product (ACI cookie, NDFC token, Intersight API key) |

## DB Regeneration (dev only)

```sh
ASSETS_DIR=../assets DB=./internal/embeddb/api.db ./scripts/ingest_all.sh
```

Env vars: `ASSETS_DIR`, `DB`, `INGEST`, `ACI_AUX_DIR`.

After ingestion, rebuild the binary to embed the updated `api.db`.

## Key Patterns

- **FTS5 + synonyms**: Queries expand terms via `synonyms` table (e.g. `vrf` → `virtual routing forwarding vrf-context`) before hitting `endpoints_fts`.
- **Temp-file SQLite**: Embedded DB bytes are written to `/tmp` at startup; opened read-only (`mode=ro`).
- **Token-efficient output**: Tools return formatted plain text, not JSON, to minimize LLM context usage.
- **Product aliases**: `ucs`/`dcnm` resolve to `intersight`/`ndfc` internally.

## Products

| Slug | Full Name | Ingestion Source |
|------|-----------|-----------------|
| `aci` | Application Centric Infrastructure | APIC JSON metadata + class JSON |
| `ndfc` | Nexus Dashboard Fabric Controller | OpenAPI 3 JSON |
| `intersight` | Cisco Intersight / UCS | OpenAPI 3 YAML |
