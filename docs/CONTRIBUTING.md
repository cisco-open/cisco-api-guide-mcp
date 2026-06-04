# How to Contribute

Thanks for your interest in contributing to `cisco-api-guide-mcp`! Here are a few general guidelines on contributing and
reporting bugs that we ask you to review. Following these guidelines helps to communicate that you respect the time of
the contributors managing and developing this open source project. In return, they should reciprocate that respect in
addressing your issue, assessing changes, and helping you finalize your pull requests. In that spirit of mutual respect,
we endeavor to review incoming issues and pull requests within 10 days, and will close any lingering issues or pull
requests after 60 days of inactivity.

Please note that all of your interactions in the project are subject to our [Code of Conduct](/CODE_OF_CONDUCT.md). This
includes creation of issues or pull requests, commenting on issues or pull requests, and extends to all interactions in
any real-time space e.g., Slack, Discord, etc.

## Reporting Issues

Before reporting a new issue, please ensure that the issue was not already reported or fixed by searching through our
[issues list](https://github.com/brightpuddle/cisco-api-guide-mcp/issues).

When creating a new issue, please be sure to include a **title and clear description**, as much relevant information as
possible, and, if possible, a test case.

**If you discover a security bug, please do not report it through GitHub. Instead, please see security procedures in
[SECURITY.md](/SECURITY.md).**

## Sending Pull Requests

Before sending a new pull request, take a look at existing pull requests and issues to see if the proposed change or fix
has been discussed in the past, or if the change was already implemented but not yet released.

We expect new pull requests to include tests for any affected behavior, and, as we follow semantic versioning, we may
reserve breaking changes until the next major version release.

## Other Ways to Contribute

We welcome anyone that wants to contribute to `cisco-api-guide-mcp` to triage and reply to open issues to help troubleshoot
and fix existing bugs. Here is what you can do:

- Help ensure that existing issues follows the recommendations from the _[Reporting Issues](#reporting-issues)_ section,
  providing feedback to the issue's author on what might be missing.
- Review and update the existing content of our [Wiki](https://github.com/brightpuddle/cisco-api-guide-mcp/wiki) with up-to-date
  instructions and code samples.
- Review existing pull requests, and testing patches against real existing applications that use `cisco-api-guide-mcp`.
- Write a test, or add a missing test case to an existing test.

## Updating API Data: The Ingestion Tool

The API database embedded in the released binary is built using a developer-only ingestion tool
located at `cmd/ingest`. Maintainers run this tool to add or update product content in
`data/api.db`, copy the result to `internal/embeddb/api.db`, and commit both files before cutting
a new release.

### Build the ingestion tool

```sh
go build -o cisco-api-guide-ingest ./cmd/ingest
```

### Seed a new product

Use `--init` to insert or update a product's metadata row before ingesting endpoints:

```sh
./cisco-api-guide-ingest \
  --db ./data/api.db \
  --product aci \
  --init \
  --name "Cisco ACI" \
  --description "Application Centric Infrastructure REST API" \
  --base-url "https://<apic>/api" \
  --auth-type "token" \
  --auth-notes "Obtain a token via POST /api/aaaLogin.json before calling other endpoints." \
  --alias "aci"
```

### Ingest endpoints

Endpoints are loaded from a JSON file using `--format manual`. The input must be a JSON object
with a top-level `"endpoints"` array:

```json
{
  "endpoints": [
    {
      "method": "GET",
      "path": "/api/node/class/{class}.json",
      "summary": "Query objects by class",
      "description": "Returns all objects of the given class.",
      "tags": ["query"],
      "parameters": [
        {
          "name": "class",
          "in": "path",
          "required": true,
          "description": "MO class name",
          "schema": { "type": "string" },
          "example": "fvTenant"
        }
      ],
      "request_body": null,
      "responses": {
        "200": { "description": "OK" }
      }
    }
  ]
}
```

Run the ingestion:

```sh
./cisco-api-guide-ingest \
  --db ./data/api.db \
  --product aci \
  --release "6.0" \
  --format manual \
  --input ./aci-endpoints.json
```

### CLI flags reference

| Flag           | Description                                                                 |
|----------------|-----------------------------------------------------------------------------|
| `--db`         | Path to the SQLite database file. Default: `./data/api.db`.                |
| `--product`    | Product slug (`aci`, `ndfc`, `intersight`).                                 |
| `--release`    | Release or version tag to attach to ingested endpoints (e.g. `"6.0"`).     |
| `--format`     | Input format. Supported: `manual`. (`openapi3`, `swagger2` are planned.)   |
| `--input`      | Path to the input file, or `-` to read from stdin.                          |
| `--synonyms`   | Path to a CSV file of `term,expansion` pairs to load into the synonym table.|
| `--init`       | Seed product metadata only (no endpoints). Use with `--name`, `--description`, etc. |
| `--name`       | Product display name (used with `--init`).                                  |
| `--description`| Product description (used with `--init`).                                  |
| `--base-url`   | API base URL (used with `--init`).                                          |
| `--auth-type`  | Authentication type (used with `--init`).                                   |
| `--auth-notes` | Authentication notes shown to AI assistants (used with `--init`).          |
| `--alias`      | Comma-separated product aliases to register (used with `--init`).           |

### Load synonyms

The search engine supports synonym expansion. Load synonyms from a two-column CSV
(`term,expansion`) to improve query recall:

```sh
./cisco-api-guide-ingest --db ./data/api.db --synonyms ./synonyms.csv
```

### Publish the updated database

After ingesting, copy the updated database to the embedded path and commit both files:

```sh
cp ./data/api.db ./internal/embeddb/api.db
git add data/api.db internal/embeddb/api.db
git commit -m "chore: update embedded API database"
```

The next release build will embed the updated database into the binary automatically.

Thanks again for your interest on contributing to `cisco-api-guide-mcp`!
