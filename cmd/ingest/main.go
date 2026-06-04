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
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	idb "github.com/brightpuddle/cisco-api-guide-mcp/internal/db"
	"github.com/brightpuddle/cisco-api-guide-mcp/cmd/ingest/formats"
)

func main() {
	app := &cli.App{
		Name:  "cisco-api-guide-ingest",
		Usage: "Ingest Cisco API documentation into the SQLite database",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Value: "./data/api.db", Usage: "Path to SQLite DB file"},
			&cli.StringFlag{Name: "product", Usage: "Product slug (aci, ndfc, intersight)"},
			&cli.StringFlag{Name: "format", Usage: "Input format: openapi3, swagger2, manual"},
			&cli.StringFlag{Name: "input", Usage: "Input file path (or - for stdin)"},
			&cli.StringFlag{Name: "synonyms", Usage: "CSV file of term,expansion pairs"},
			&cli.BoolFlag{Name: "init", Usage: "Seed product metadata row only"},
			&cli.StringFlag{Name: "name", Usage: "Product display name (used with --init)"},
			&cli.StringFlag{Name: "description", Usage: "Product description (used with --init)"},
			&cli.StringFlag{Name: "base-url", Usage: "API base URL (used with --init)"},
			&cli.StringFlag{Name: "auth-type", Usage: "Auth type (used with --init)"},
			&cli.StringFlag{Name: "auth-notes", Usage: "Auth notes (used with --init)"},
			&cli.StringFlag{Name: "alias", Usage: "Comma-separated aliases to register for product (used with --init)"},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx *cli.Context) error {
	dbPath := ctx.String("db")
	db, err := idb.OpenRW(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := applySchema(db); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	if ctx.String("synonyms") != "" {
		return loadSynonyms(db, ctx.String("synonyms"))
	}

	product := ctx.String("product")
	if product == "" {
		return fmt.Errorf("--product is required")
	}

	if ctx.Bool("init") {
		return seedProduct(db, ctx)
	}

	format := ctx.String("format")
	if format == "" {
		return fmt.Errorf("--format is required")
	}
	handler, ok := formats.Handlers[format]
	if !ok {
		return fmt.Errorf("unsupported format %q", format)
	}

	inputPath := ctx.String("input")
	var data []byte
	if inputPath == "-" || inputPath == "" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(inputPath)
	}
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	endpoints, err := handler.Parse(product, data)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	return insertEndpoints(db, endpoints)
}

func applySchema(db *sql.DB) error {
	schema, err := os.ReadFile("internal/db/schema.sql")
	if err != nil {
		// Try relative to binary location
		schema, err = os.ReadFile("schema.sql")
		if err != nil {
			return fmt.Errorf("read schema: %w", err)
		}
	}
	_, err = db.Exec(string(schema))
	return err
}

func seedProduct(db *sql.DB, ctx *cli.Context) error {
	product := ctx.String("product")
	name := ctx.String("name")
	if name == "" {
		name = product
	}

	_, err := db.Exec(`
		INSERT INTO products(id, name, description, base_url, auth_type, auth_notes, auth_schema)
		VALUES(?, ?, ?, ?, ?, ?, '{}')
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			description=excluded.description,
			base_url=excluded.base_url,
			auth_type=excluded.auth_type,
			auth_notes=excluded.auth_notes`,
		product, name,
		ctx.String("description"),
		ctx.String("base-url"),
		ctx.String("auth-type"),
		ctx.String("auth-notes"),
	)
	if err != nil {
		return fmt.Errorf("insert product: %w", err)
	}

	for _, alias := range strings.Split(ctx.String("alias"), ",") {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		_, err := db.Exec(`
			INSERT INTO product_aliases(alias, product_id) VALUES(?, ?)
			ON CONFLICT(alias) DO UPDATE SET product_id=excluded.product_id`,
			alias, product)
		if err != nil {
			return fmt.Errorf("insert alias %q: %w", alias, err)
		}
	}

	fmt.Printf("Seeded product %q\n", product)
	return nil
}

func loadSynonyms(db *sql.DB, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open synonyms: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read csv: %w", err)
		}
		if len(record) < 2 {
			continue
		}
		_, err = db.Exec(`INSERT INTO synonyms(term, expansion) VALUES(?, ?)`, record[0], record[1])
		if err != nil {
			return fmt.Errorf("insert synonym: %w", err)
		}
		count++
	}
	fmt.Printf("Loaded %d synonyms\n", count)
	return nil
}

func insertEndpoints(db *sql.DB, endpoints []idb.Endpoint) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO endpoints(product_id, method, path, summary, description, tags, parameters, request_body, responses, source_format)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_id, method, path) DO UPDATE SET
			summary=excluded.summary,
			description=excluded.description,
			tags=excluded.tags,
			parameters=excluded.parameters,
			request_body=excluded.request_body,
			responses=excluded.responses,
			source_format=excluded.source_format`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range endpoints {
		_, err := stmt.Exec(e.ProductID, e.Method, e.Path, e.Summary, e.Description,
			e.Tags, e.Parameters, e.RequestBody, e.Responses, e.SourceFormat)
		if err != nil {
			return fmt.Errorf("insert endpoint %s %s: %w", e.Method, e.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	fmt.Printf("Inserted %d endpoints\n", len(endpoints))
	return nil
}
