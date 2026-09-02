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

package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// ResolveProduct maps alias → canonical product id.
func ResolveProduct(db *sql.DB, slug string) (string, error) {
	// Check direct match
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM products WHERE id = ?`, slug).Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		return slug, nil
	}
	// Check aliases
	var productID string
	err = db.QueryRow(`SELECT product_id FROM product_aliases WHERE alias = ?`, slug).Scan(&productID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("unknown product %q", slug)
	}
	return productID, err
}

// GetProduct returns full product row.
func GetProduct(db *sql.DB, id string) (*Product, error) {
	p := &Product{}
	err := db.QueryRow(`
		SELECT id, name, description, base_url, auth_type, auth_notes, auth_schema
		FROM products WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.BaseURL, &p.AuthType, &p.AuthNotes, &p.AuthSchema)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("product %q not found", id)
	}
	return p, err
}

// SearchEndpoints runs FTS5 query and returns ranked results.
// releasePrefix filters by release using prefix matching (e.g. "3" matches "3.2.2m").
// Empty productID or releasePrefix disables that filter.
func SearchEndpoints(db *sql.DB, ftsQuery, productID, releasePrefix string, limit int) ([]SearchResult, int, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	where := "endpoints_fts MATCH ?"
	args := []interface{}{ftsQuery}

	if productID != "" {
		where += " AND e.product_id = ?"
		args = append(args, productID)
	}
	if releasePrefix != "" {
		where += " AND e.release LIKE ?"
		args = append(args, releasePrefix+"%")
	}
	args = append(args, limit+1)

	rows, err := db.Query(`
		SELECT e.product_id, e.release, e.method, e.path, e.summary
		FROM endpoints e
		JOIN endpoints_fts f ON e.id = f.rowid
		WHERE `+where+`
		ORDER BY rank
		LIMIT ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("fts query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ProductID, &r.Release, &r.Method, &r.Path, &r.Summary); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}

	total := len(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, total, rows.Err()
}

// GetEndpoint fetches full endpoint detail.
// releasePrefix filters by release using prefix matching; empty matches any release.
// If multiple releases match with no prefix, returns an error listing available releases.
func GetEndpoint(db *sql.DB, productID, releasePrefix, method, path string) (*Endpoint, error) {
	query := `
		SELECT id, product_id, release, method, path, summary, description,
		       tags, parameters, request_body, responses, source_format
		FROM endpoints
		WHERE product_id = ? AND method = ? AND path = ?`
	args := []interface{}{productID, strings.ToUpper(method), path}

	if releasePrefix != "" {
		query += " AND release LIKE ?"
		args = append(args, releasePrefix+"%")
	}
	query += " ORDER BY release"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	defer rows.Close()

	var results []*Endpoint
	for rows.Next() {
		e := &Endpoint{}
		if err := rows.Scan(&e.ID, &e.ProductID, &e.Release, &e.Method, &e.Path,
			&e.Summary, &e.Description, &e.Tags, &e.Parameters,
			&e.RequestBody, &e.Responses, &e.SourceFormat); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	switch len(results) {
	case 0:
		return nil, fmt.Errorf("endpoint not found: %s %s %s", productID, method, path)
	case 1:
		return results[0], nil
	default:
		releases := make([]string, len(results))
		for i, r := range results {
			releases[i] = fmt.Sprintf("%q", r.Release)
		}
		return nil, fmt.Errorf(
			"multiple releases match for %s %s %s: [%s] — specify a release parameter",
			productID, method, path, strings.Join(releases, ", "))
	}
}

// ProductRelease describes one product/release pairing available in a module
// database, along with how many endpoints are indexed under it.
type ProductRelease struct {
	ProductID     string
	Release       string
	EndpointCount int
}

// ListProductReleases returns the distinct (product_id, release) pairs present
// in the endpoints table, along with endpoint counts, ordered by product_id
// then release.
func ListProductReleases(db *sql.DB) ([]ProductRelease, error) {
	rows, err := db.Query(`
		SELECT product_id, release, COUNT(*)
		FROM endpoints
		GROUP BY product_id, release
		ORDER BY product_id, release`)
	if err != nil {
		return nil, fmt.Errorf("list product releases: %w", err)
	}
	defer rows.Close()

	var out []ProductRelease
	for rows.Next() {
		var pr ProductRelease
		if err := rows.Scan(&pr.ProductID, &pr.Release, &pr.EndpointCount); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// SearchNACPaths runs FTS5 query against nac_paths and returns ranked results.
// releasePrefix filters by release using prefix matching (e.g. "3" matches "3.2.2m").
// Empty productID or releasePrefix disables that filter.
func SearchNACPaths(db *sql.DB, ftsQuery, productID, releasePrefix string, limit int) ([]NACSearchResult, int, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	where := "nac_paths_fts MATCH ?"
	args := []interface{}{ftsQuery}

	if productID != "" {
		where += " AND n.product_id = ?"
		args = append(args, productID)
	}
	if releasePrefix != "" {
		where += " AND n.release LIKE ?"
		args = append(args, releasePrefix+"%")
	}
	args = append(args, limit+1)

	rows, err := db.Query(`
		SELECT n.product_id, n.release, n.path, n.object_name, n.description
		FROM nac_paths n
		JOIN nac_paths_fts f ON n.id = f.rowid
		WHERE `+where+`
		ORDER BY rank
		LIMIT ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("fts query: %w", err)
	}
	defer rows.Close()

	var results []NACSearchResult
	for rows.Next() {
		var r NACSearchResult
		if err := rows.Scan(&r.ProductID, &r.Release, &r.Path, &r.ObjectName, &r.Description); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}

	total := len(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, total, rows.Err()
}

// GetNACPath fetches full NAC path detail.
// releasePrefix filters by release using prefix matching; empty matches any release.
// If multiple releases match with no prefix, returns an error listing available releases.
func GetNACPath(db *sql.DB, productID, releasePrefix, path string) (*NACPath, error) {
	query := `
		SELECT id, product_id, release, path, object_name, gui_location,
		       description, schema, examples, source_format
		FROM nac_paths
		WHERE product_id = ? AND path = ?`
	args := []interface{}{productID, path}

	if releasePrefix != "" {
		query += " AND release LIKE ?"
		args = append(args, releasePrefix+"%")
	}
	query += " ORDER BY release"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get nac path: %w", err)
	}
	defer rows.Close()

	var results []*NACPath
	for rows.Next() {
		n := &NACPath{}
		if err := rows.Scan(&n.ID, &n.ProductID, &n.Release, &n.Path, &n.ObjectName,
			&n.GUILocation, &n.Description, &n.Schema, &n.Examples, &n.SourceFormat); err != nil {
			return nil, err
		}
		results = append(results, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	switch len(results) {
	case 0:
		return nil, fmt.Errorf("nac path not found: %s %s", productID, path)
	case 1:
		return results[0], nil
	default:
		releases := make([]string, len(results))
		for i, r := range results {
			releases[i] = fmt.Sprintf("%q", r.Release)
		}
		return nil, fmt.Errorf(
			"multiple releases match for %s %s: [%s] — specify a release parameter",
			productID, path, strings.Join(releases, ", "))
	}
}

// NACPathRelease describes one product/release pairing available in a module
// database, along with how many NAC paths are indexed under it.
type NACPathRelease struct {
	ProductID string
	Release   string
	PathCount int
}

// ListNACPathReleases returns the distinct (product_id, release) pairs present
// in the nac_paths table, along with path counts, ordered by product_id then
// release.
func ListNACPathReleases(db *sql.DB) ([]NACPathRelease, error) {
	rows, err := db.Query(`
		SELECT product_id, release, COUNT(*)
		FROM nac_paths
		GROUP BY product_id, release
		ORDER BY product_id, release`)
	if err != nil {
		return nil, fmt.Errorf("list nac path releases: %w", err)
	}
	defer rows.Close()

	var out []NACPathRelease
	for rows.Next() {
		var pr NACPathRelease
		if err := rows.Scan(&pr.ProductID, &pr.Release, &pr.PathCount); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// GetSynonyms returns all synonym rows.
func GetSynonyms(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT term, expansion FROM synonyms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var term, exp string
		if err := rows.Scan(&term, &exp); err != nil {
			return nil, err
		}
		m[term] = exp
	}
	return m, rows.Err()
}
