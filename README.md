# Cisco API Guide MCP Server

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server that
gives AI assistants searchable, token-efficient access to Cisco product API
documentation.

## Overview

`cisco-api-guide-mcp` ships as a lightweight, modular binary that downloads and caches
SQLite databases on demand for:

- **Cisco ACI** — Application Centric Infrastructure (APIC REST API)
- **Cisco NDFC** — Nexus Dashboard Fabric Controller (formerly DCNM)
- **Cisco Intersight / UCS** — Unified Compute System & Intersight SaaS REST API

The server speaks the [MCP protocol](https://modelcontextprotocol.io) over stdio
(JSON-RPC 2.0) or streamable HTTP and exposes three tools that an AI assistant can
call to search endpoints, retrieve full endpoint details, and look up product
authentication guides.

## Quick Start

### 1. Direct Binary Installation (using `go install`)

If you have Go installed (1.21+):

```sh
go install github.com/cisco-open/cisco-api-guide-mcp/cmd/cisco-api-guide@latest
```

The `cisco-api-guide` binary will be installed to your `$GOPATH/bin` (or `~/go/bin`). Ensure that directory is in your `PATH`.

### 2. Download Pre-built Binary

Download the latest release for your platform from the
[GitHub Releases page](https://github.com/cisco-open/cisco-api-guide-mcp/releases):

| Platform              | Archive                             |
| --------------------- | ----------------------------------- |
| macOS (Apple Silicon) | `cisco-api-guide_darwin_arm64.zip`  |
| Linux (x86-64)        | `cisco-api-guide_linux_amd64.zip`   |
| Windows (x86-64)      | `cisco-api-guide_windows_amd64.zip` |

Extract the archive and place the `cisco-api-guide` binary somewhere on your `PATH`.

### 3. Build from Source

```sh
git clone https://github.com/cisco-open/cisco-api-guide-mcp.git
cd cisco-api-guide-mcp
go build -o bin/cisco-api-guide ./cmd/cisco-api-guide
```

### 4. Configure Your MCP Client

The server runs as a stdio process. Add it to your MCP client configuration:

**Claude Desktop**
(`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS,
`%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "cisco-api-guide": {
      "command": "cisco-api-guide",
      "args": ["--modules", "all"]
    }
  }
}
```

**OpenCode / Cursor / VS Code**:

```json
{
  "mcp": {
    "cisco-api-guide": {
      "type": "stdio",
      "command": "cisco-api-guide",
      "args": ["--modules", "aci,ndfc"]
    }
  }
}
```

## CLI Configuration & Modular APIs

The server downloads only the modules you configure, caching them in `~/.cache/cisco-api-guide/` (or OS standard cache path):

```sh
# Load specific products
cisco-api-guide --modules aci,ndfc

# Load all available products (default)
cisco-api-guide --modules all

# Custom cache storage directory
cisco-api-guide --data-dir /custom/cache/path

# Check remote manifest and auto-update cached DBs on launch
cisco-api-guide --auto-update

# Run as a streamable HTTP server instead of stdio
cisco-api-guide --http --addr :8080
```

Environment variables are also supported:
- `CISCO_API_MODULES`: Comma-separated product list (e.g. `aci,ndfc`)
- `CISCO_API_GUIDE_DATA_DIR`: Custom cache directory path
- `CISCO_API_REGISTRY_URL`: URL to custom `modules.json` registry manifest

## Exposed MCP Tools

### `search_endpoints`

Search Cisco API endpoints by natural language or keywords. Returns a ranked
list of matching endpoints.

| Parameter | Type    | Required | Description                                                                               |
| --------- | ------- | -------- | ----------------------------------------------------------------------------------------- |
| `query`   | string  | Yes      | Natural language or keyword query. Example: `"query VRFs in a tenant"` or `"create EPG"`. |
| `product` | string  | No       | Filter to a specific product. One of: `aci`, `ndfc`, `intersight`, `ucs`, `dcnm`.         |
| `release` | string  | No       | Filter by release using prefix matching. Example: `"3"` matches `"3.2.2m"`.               |
| `limit`   | integer | No       | Maximum results to return. Default `10`, max `50`.                                        |

### `get_endpoint`

Get full detail for a specific Cisco API endpoint, including parameters, request
body schema, and responses.

| Parameter | Type   | Required | Description                                                                            |
| --------- | ------ | -------- | -------------------------------------------------------------------------------------- |
| `product` | string | Yes      | Product slug. One of: `aci`, `ndfc`, `intersight`, `ucs`, `dcnm`.                      |
| `method`  | string | Yes      | HTTP method. One of: `GET`, `POST`, `PUT`, `DELETE`, `PATCH`.                          |
| `path`    | string | Yes      | API path as returned by `search_endpoints`. Example: `"/api/node/class/{class}.json"`. |
| `release` | string | No       | Release prefix to select a specific version. Required when multiple releases match.    |

### `get_product_guide`

Get authentication instructions and general usage notes for a Cisco product API.

| Parameter | Type   | Required | Description                                                       |
| --------- | ------ | -------- | ----------------------------------------------------------------- |
| `product` | string | Yes      | Product slug. One of: `aci`, `ndfc`, `intersight`, `ucs`, `dcnm`. |

## Adding or Updating API Modules

To add a new Cisco API or update an existing release:

1. **Add the raw spec to `assets/`**:
   - Place OpenAPI (JSON/YAML) or class metadata under `assets/<product>/<release>/...` (e.g. `assets/ndfc/4.1.1/infra.json`).
2. **Build and test the database locally**:
   ```sh
   ./scripts/ingest_all.sh
   go test ./...
   ```
3. **Submit a Pull Request**:
   - Commit only the text files in `assets/` (no binary SQLite databases are stored in Git).
   - Once merged, the GitHub Actions CI pipeline automatically compiles the SQLite databases, computes SHA-256 hashes, and publishes the release artifacts.

## Contributing

Contributions are welcome. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for
guidelines on reporting issues, submitting pull requests, and adding new API
modules.

For security vulnerabilities, follow the responsible disclosure process
described in [SECURITY.md](SECURITY.md).

## License

SPDX-License-Identifier: Apache-2.0

Copyright 2026 Cisco Systems, Inc. and their affiliates

Licensed under the Apache License, Version 2.0 (the "License"); you may not use
this file except in compliance with the License. You may obtain a copy of the
License at

<http://www.apache.org/licenses/LICENSE-2.0>

Unless required by applicable law or agreed to in writing, software distributed
under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the
specific language governing permissions and limitations under the License.
