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

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
	"github.com/cisco-open/cisco-api-guide-mcp/cmd/ingest/formats"
)

func main() {
	app := &cli.App{
		Name:  "cisco-api-guide-ingest",
		Usage: "Ingest Cisco API documentation into the SQLite database",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Value: "./internal/embeddb/api.db", Usage: "Path to SQLite DB file"},
			&cli.StringFlag{Name: "product", Usage: "Product slug (aci, ndfc, intersight)"},
			&cli.StringFlag{Name: "release", Usage: "Release/version tag for this endpoint set (e.g. 3.2.2m, 4.0.0)"},
			&cli.StringFlag{Name: "format", Usage: "Input format: openapi3, aci-meta, swagger2, manual, nac-schema"},
			&cli.StringFlag{Name: "input", Usage: "Input file path (or - for stdin)"},
			&cli.StringFlag{Name: "aux-dir", Usage: "Auxiliary directory (e.g. APIC per-class JSON docs for aci-meta format)"},
			&cli.BoolFlag{Name: "prune-major", Usage: "Delete existing endpoints for same product+major-version before inserting"},
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

	release := ctx.String("release")

	if nacHandler, ok := formats.NACHandlers[format]; ok {
		if h, ok := nacHandler.(*formats.NACSchemaHandler); ok {
			h.AuxDir = ctx.String("aux-dir")
		}

		paths, err := nacHandler.Parse(product, data)
		if err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		for i := range paths {
			paths[i].Release = release
		}

		if ctx.Bool("prune-major") && release != "" {
			if err := pruneReleaseMajorTable(db, "nac_paths", product, release); err != nil {
				return fmt.Errorf("prune major release: %w", err)
			}
		}

		return insertNACPaths(db, paths)
	}

	handler, ok := formats.Handlers[format]
	if !ok {
		return fmt.Errorf("unsupported format %q", format)
	}

	// Allow format handlers to receive extra options via type assertion.
	if h, ok := handler.(*formats.ACIMetaHandler); ok {
		h.AuxDir = ctx.String("aux-dir")
	}

	endpoints, err := handler.Parse(product, data)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	for i := range endpoints {
		endpoints[i].Release = release
	}

	if ctx.Bool("prune-major") && release != "" {
		if err := pruneReleaseMajorTable(db, "endpoints", product, release); err != nil {
			return fmt.Errorf("prune major release: %w", err)
		}
	}

	return insertEndpoints(db, endpoints)
}

func applySchema(db *sql.DB) error {
	if err := migrateSchema(db); err != nil {
		return fmt.Errorf("schema migration: %w", err)
	}

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

// migrateSchema detects and applies incremental schema changes that cannot be
// handled by idempotent CREATE TABLE IF NOT EXISTS statements (e.g. adding
// columns or changing UNIQUE constraints on existing tables).
func migrateSchema(db *sql.DB) error {
	// Nothing to do if the endpoints table doesn't exist yet.
	var count int
	if err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='endpoints'`,
	).Scan(&count); err != nil || count == 0 {
		return nil
	}

	// v0 -> v1: add 'release' column + fix UNIQUE constraint + rebuild FTS.
	if err := migrateV0toV1(db); err != nil {
		return fmt.Errorf("v0->v1: %w", err)
	}
	return nil
}

// migrateV0toV1 adds the 'release' column to the endpoints table and updates
// the UNIQUE constraint from (product_id, method, path) to
// (product_id, release, method, path). It is a no-op if the column already exists.
func migrateV0toV1(db *sql.DB) error {
	// Check whether the column already exists.
	rows, err := db.Query(`PRAGMA table_info(endpoints)`)
	if err != nil {
		return fmt.Errorf("pragma table_info: %w", err)
	}
	hasRelease := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err == nil {
			if name == "release" {
				hasRelease = true
			}
		}
	}
	rows.Close()

	if hasRelease {
		return nil // already migrated
	}

	fmt.Println("Migrating endpoints schema (v0->v1: adding release column)…")

	// Each step is a separate Exec so errors are attributable.
	steps := []string{
		// New table with the correct schema.
		`CREATE TABLE endpoints_new (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id    TEXT NOT NULL REFERENCES products(id),
			release       TEXT NOT NULL DEFAULT '',
			method        TEXT NOT NULL,
			path          TEXT NOT NULL,
			summary       TEXT NOT NULL DEFAULT '',
			description   TEXT NOT NULL DEFAULT '',
			tags          TEXT NOT NULL DEFAULT '[]',
			parameters    TEXT NOT NULL DEFAULT '[]',
			request_body  TEXT NOT NULL DEFAULT '{}',
			responses     TEXT NOT NULL DEFAULT '{}',
			source_format TEXT NOT NULL DEFAULT '',
			UNIQUE(product_id, release, method, path)
		)`,

		// Copy existing rows; release defaults to empty string.
		`INSERT INTO endpoints_new
			(id, product_id, release, method, path, summary, description,
			 tags, parameters, request_body, responses, source_format)
		 SELECT
			id, product_id, '', method, path, summary, description,
			tags, parameters, request_body, responses, source_format
		 FROM endpoints`,

		// Drop FTS triggers before dropping the source table.
		`DROP TRIGGER IF EXISTS endpoints_fts_insert`,
		`DROP TRIGGER IF EXISTS endpoints_fts_delete`,
		`DROP TRIGGER IF EXISTS endpoints_fts_update`,
		`DROP TABLE IF EXISTS endpoints_fts`,

		// Swap tables.
		`DROP TABLE endpoints`,
		`ALTER TABLE endpoints_new RENAME TO endpoints`,

		// Rebuild FTS content table (triggers recreated by schema.sql).
		`CREATE VIRTUAL TABLE endpoints_fts USING fts5(
			summary, description, path, tags,
			content='endpoints', content_rowid='id',
			tokenize='porter unicode61'
		)`,
		`INSERT INTO endpoints_fts(rowid, summary, description, path, tags)
		 SELECT id, summary, description, path, tags FROM endpoints`,
	}

	for _, stmt := range steps {
		if _, err := db.Exec(stmt); err != nil {
			preview := stmt
			if len(preview) > 60 {
				preview = preview[:60] + "…"
			}
			return fmt.Errorf("step %q: %w", preview, err)
		}
	}

	fmt.Println("Schema migration complete.")
	return nil
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

// pruneReleaseMajorTable deletes all rows in the given table for the given
// product whose release shares the same major-version prefix as newRelease.
//
// Examples: newRelease="4.1.1" -> deletes releases "4", "4.0.0", "4.1.0", etc.
//           newRelease="3.2.2m" -> deletes releases "3", "3.0.0", "3.2.2m", etc.
//
// table must be a trusted, hardcoded caller-supplied identifier (never derived
// from user input) since it cannot be parameterized in the DELETE statement.
func pruneReleaseMajorTable(db *sql.DB, table, productID, newRelease string) error {
	major := strings.SplitN(newRelease, ".", 2)[0]
	res, err := db.Exec(
		`DELETE FROM `+table+` WHERE product_id = ? AND (release = ? OR release LIKE ?)`,
		productID, major, major+".%",
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		fmt.Printf("Pruned %d rows from %s for %s major version %s\n", n, table, productID, major)
	}
	return nil
}

func insertEndpoints(db *sql.DB, endpoints []idb.Endpoint) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO endpoints(product_id, release, method, path, summary, description, tags, parameters, request_body, responses, source_format)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_id, release, method, path) DO UPDATE SET
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
		_, err := stmt.Exec(e.ProductID, e.Release, e.Method, e.Path, e.Summary, e.Description,
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

func insertNACPaths(db *sql.DB, paths []idb.NACPath) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO nac_paths(product_id, release, path, object_name, gui_location, description, schema, examples, source_format)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_id, release, path) DO UPDATE SET
			object_name=excluded.object_name,
			gui_location=excluded.gui_location,
			description=excluded.description,
			schema=excluded.schema,
			examples=excluded.examples,
			source_format=excluded.source_format`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, p := range paths {
		_, err := stmt.Exec(p.ProductID, p.Release, p.Path, p.ObjectName, p.GUILocation,
			p.Description, p.Schema, p.Examples, p.SourceFormat)
		if err != nil {
			return fmt.Errorf("insert nac path %s: %w", p.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	fmt.Printf("Inserted %d nac paths\n", len(paths))
	return nil
}
