# Cisco API Guide MCP Server

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server that
gives AI assistants searchable, token-efficient access to Cisco product API
documentation.

## Overview

`cisco-api-guide-mcp` ships as a single self-contained binary with an embedded
SQLite database of API endpoints, authentication guides, and reference content
for:

- **Cisco ACI** — Application Centric Infrastructure
- **Cisco NDFC** — Nexus Dashboard Fabric Controller (formerly DCNM)
- **Cisco Intersight / UCS** — Unified Compute System

The server speaks the [MCP protocol](https://modelcontextprotocol.io) over stdio
(JSON-RPC 2.0) and exposes three tools that an AI assistant can call to search
endpoints, retrieve full endpoint details, and look up product authentication
guides. It is intended for AI developers building automation, integrations, and
tooling against Cisco infrastructure APIs.

## Quick Start

### 1. Download the binary

Download the latest release for your platform from the
[GitHub Releases page](https://github.com/brightpuddle/cisco-api-guide-mcp/releases):

| Platform              | Archive                             |
| --------------------- | ----------------------------------- |
| macOS (Apple Silicon) | `cisco-api-guide_darwin_arm64.zip`  |
| Linux (x86-64)        | `cisco-api-guide_linux_amd64.zip`   |
| Windows (x86-64)      | `cisco-api-guide_windows_amd64.zip` |

Extract the archive and place the `cisco-api-guide` binary somewhere on your
`PATH`.

### 2. Build from source

Requires Go 1.21+:

```sh
git clone https://github.com/brightpuddle/cisco-api-guide-mcp.git
cd cisco-api-guide-mcp
go build -o cisco-api-guide ./cmd/cisco-api-guide
```

> **Note:** The build embeds the bundled API database at
> `internal/embeddb/api.db`. No external database or network access is required
> at runtime.

### 3. Configure your MCP client

The server runs as a stdio process. Add it to your MCP client configuration:

**Claude Desktop**
(`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS,
`%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "cisco-api-guide": {
      "command": "/usr/local/bin/cisco-api-guide"
    }
  }
}
```

**OpenCode** (`opencode.json` in your project or
`~/.config/opencode/config.json`):

```json
{
  "mcp": {
    "cisco-api-guide": {
      "type": "stdio",
      "command": "/usr/local/bin/cisco-api-guide"
    }
  }
}
```

Any MCP-compatible client can be configured the same way: point `command` at the
binary and use stdio transport.

## Tools

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

## Contributing

Contributions are welcome. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for
guidelines on reporting issues, submitting pull requests, and updating the API
database using the ingestion tool.

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
