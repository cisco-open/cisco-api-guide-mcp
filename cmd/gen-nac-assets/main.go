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

// Command gen-nac-assets generates the assets/nac-<product>/schema.json and
// assets/nac-<product>/docs/*.json files consumed by `cisco-api-guide-ingest
// --format nac-schema`, sourced live from:
//   - the public schemastore-cataloged NaC JSON Schema
//     (https://raw.githubusercontent.com/netascode/schema/main/schema.json)
//   - the internal wwwin-github.cisco.com netascode/nac-<product> repo's
//     docs/templates/<solution>/<folder>/<object>.md files
//
// This is a manual maintainer command: run it, review the generated
// gen-report.txt for unmatched docs/schema paths, then commit the output
// under assets/.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/urfave/cli/v2"
)

// nacSchemaEntry mirrors cmd/ingest/formats/nac.go's unexported type of the
// same name — the exact shape NACSchemaHandler.Parse expects in --input.
type nacSchemaEntry struct {
	Path       string          `json:"path"`
	ObjectName string          `json:"object_name"`
	Schema     json.RawMessage `json:"schema"`
}

// nacAuxDoc mirrors cmd/ingest/formats/nac.go's unexported type of the same
// name — the exact per-path shape loadNACAuxDocs expects in --aux-dir files.
type nacAuxDoc struct {
	ObjectName  string          `json:"object_name"`
	GUILocation string          `json:"gui_location"`
	Description string          `json:"description"`
	Examples    []nacAuxExample `json:"examples"`
}

func main() {
	app := &cli.App{
		Name:  "gen-nac-assets",
		Usage: "Generate assets/nac-<product>/{schema.json,docs/} from live NaC schema + docs sources",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "product", Required: true, Usage: "nac-aci or nac-vxlan"},
			&cli.StringFlag{Name: "solution", Usage: "Schema/docs solution key (default: apic for nac-aci, vxlan for nac-vxlan)"},
			&cli.StringFlag{Name: "schema-url", Value: "https://raw.githubusercontent.com/netascode/schema/main/schema.json"},
			&cli.StringFlag{Name: "docs-repo", Usage: "GHE org/repo (default: netascode/nac-aci or netascode/nac-vxlan)"},
			&cli.StringFlag{Name: "docs-branch", Usage: "Docs repo branch (default: master for nac-aci, develop for nac-vxlan)"},
			&cli.StringFlag{Name: "ghe-base-url", Value: "https://wwwin-github.cisco.com"},
			&cli.StringFlag{Name: "ghe-token", EnvVars: []string{"GHE_TOKEN"}, Usage: "Optional GHE token (raises rate limits; repos are reachable unauthenticated)"},
			&cli.StringFlag{Name: "out", Usage: "Output directory (default: assets/<product>)"},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx *cli.Context) error {
	product := ctx.String("product")

	solution := ctx.String("solution")
	docsRepo := ctx.String("docs-repo")
	docsBranch := ctx.String("docs-branch")

	switch product {
	case "nac-aci":
		if solution == "" {
			solution = "apic"
		}
		if docsRepo == "" {
			docsRepo = "netascode/nac-aci"
		}
		if docsBranch == "" {
			docsBranch = "master"
		}
	case "nac-vxlan":
		if solution == "" {
			solution = "vxlan"
		}
		if docsRepo == "" {
			docsRepo = "netascode/nac-vxlan"
		}
		if docsBranch == "" {
			docsBranch = "develop"
		}
	default:
		return fmt.Errorf("--product must be nac-aci or nac-vxlan, got %q", product)
	}

	out := ctx.String("out")
	if out == "" {
		out = filepath.Join("assets", product)
	}

	gheBaseURL := ctx.String("ghe-base-url")
	gheToken := ctx.String("ghe-token")

	fmt.Printf("Fetching schema from %s ...\n", ctx.String("schema-url"))
	schemaDoc, err := FetchSchema(ctx.String("schema-url"))
	if err != nil {
		return err
	}

	solutionNode, err := ExtractSolution(schemaDoc, solution)
	if err != nil {
		return err
	}

	fmt.Printf("Fetching doc tree from %s/%s@%s ...\n", gheBaseURL, docsRepo, docsBranch)
	docs, truncated, err := FetchDocTree(gheBaseURL, docsRepo, docsBranch, solution, gheToken)
	if err != nil {
		return err
	}
	if truncated {
		fmt.Fprintln(os.Stderr, "warning: GHE tree listing was truncated by the API; some docs may be missing")
	}
	fmt.Printf("Found %d doc files\n", len(docs))

	folderSet := map[string]bool{}
	for _, d := range docs {
		folderSet[d.Folder] = true
	}
	folders := make([]string, 0, len(folderSet))
	for f := range folderSet {
		folders = append(folders, f)
	}
	sort.Strings(folders)

	candidates := WalkCandidates(solutionNode, solution, folders)
	fmt.Printf("Found %d schema candidates across %d folders\n", len(candidates), len(folders))

	report := MatchDocsToCandidates(candidates, docs)
	fmt.Printf("Matched %d/%d docs\n", len(report.Matched), len(docs))

	fmt.Println("Fetching matched doc content ...")
	parsed := map[string]ParsedDoc{} // keyed by candidate path
	for _, m := range report.Matched {
		content, err := FetchRaw(gheBaseURL, docsRepo, docsBranch, m.Doc.Path, gheToken)
		if err != nil {
			return err
		}
		parsed[m.Candidate.Path] = ParseMarkdown(content)
	}

	if err := writeOutput(out, solution, product, candidates, parsed, report); err != nil {
		return err
	}

	fmt.Printf("Wrote %s/schema.json (%d entries) and %s/docs/ (%d files)\n", out, len(candidates), out, len(parsed))
	fmt.Printf("See %s/gen-report.txt for the full match report.\n", out)
	return nil
}

func writeOutput(out, solution, product string, candidates []Candidate, parsed map[string]ParsedDoc, report MatchReport) error {
	if err := os.MkdirAll(filepath.Join(out, "docs"), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", out, err)
	}

	entries := make([]nacSchemaEntry, 0, len(candidates))
	for _, c := range candidates {
		entries = append(entries, nacSchemaEntry{
			Path:       c.Path,
			ObjectName: c.ObjectName,
			Schema:     c.Schema,
		})
	}

	schemaBytes, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(out, "schema.json"), schemaBytes, 0o644); err != nil {
		return fmt.Errorf("write schema.json: %w", err)
	}

	for path, p := range parsed {
		doc := nacAuxDoc{
			ObjectName:  p.ObjectName,
			GUILocation: p.GUILocation,
			Description: p.Description,
			Examples:    p.Examples,
		}
		docBytes, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal doc %s: %w", path, err)
		}
		docPath := filepath.Join(out, "docs", path+".json")
		if err := os.WriteFile(docPath, docBytes, 0o644); err != nil {
			return fmt.Errorf("write doc %s: %w", docPath, err)
		}
	}

	reportPath := filepath.Join(out, "gen-report.txt")
	if err := os.WriteFile(reportPath, []byte(WriteReport(report)), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	_ = solution
	_ = product
	return nil
}
