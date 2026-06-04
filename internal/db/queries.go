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
func SearchEndpoints(db *sql.DB, ftsQuery, productID string, limit int) ([]SearchResult, int, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	var (
		rows *sql.Rows
		err  error
	)

	if productID != "" {
		rows, err = db.Query(`
			SELECT e.product_id, e.method, e.path, e.summary
			FROM endpoints e
			JOIN endpoints_fts f ON e.id = f.rowid
			WHERE endpoints_fts MATCH ? AND e.product_id = ?
			ORDER BY rank
			LIMIT ?`, ftsQuery, productID, limit+1)
	} else {
		rows, err = db.Query(`
			SELECT e.product_id, e.method, e.path, e.summary
			FROM endpoints e
			JOIN endpoints_fts f ON e.id = f.rowid
			WHERE endpoints_fts MATCH ?
			ORDER BY rank
			LIMIT ?`, ftsQuery, limit+1)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("fts query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ProductID, &r.Method, &r.Path, &r.Summary); err != nil {
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
func GetEndpoint(db *sql.DB, productID, method, path string) (*Endpoint, error) {
	e := &Endpoint{}
	err := db.QueryRow(`
		SELECT id, product_id, method, path, summary, description,
		       tags, parameters, request_body, responses, source_format
		FROM endpoints
		WHERE product_id = ? AND method = ? AND path = ?`,
		productID, strings.ToUpper(method), path).
		Scan(&e.ID, &e.ProductID, &e.Method, &e.Path, &e.Summary, &e.Description,
			&e.Tags, &e.Parameters, &e.RequestBody, &e.Responses, &e.SourceFormat)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("endpoint not found: %s %s %s", productID, method, path)
	}
	return e, err
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
