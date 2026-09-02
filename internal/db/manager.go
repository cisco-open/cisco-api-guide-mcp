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
	"path/filepath"
	"strings"
	"sync"
)

// ModuleDB wraps a single product's SQLite database.
type ModuleDB struct {
	Path      string
	DB        *sql.DB
	ProductID string
	Aliases   []string
}

// Manager manages multiple modular product SQLite databases and provides
// unified querying, search, and resolution across all loaded modules.
type Manager struct {
	mu       sync.RWMutex
	modules  map[string]*ModuleDB // keyed by canonical product ID
	aliases  map[string]string    // alias -> canonical product ID
	synonyms map[string]string    // term -> expansion
}

// NewManager creates an empty modular DB manager.
func NewManager() *Manager {
	return &Manager{
		modules:  make(map[string]*ModuleDB),
		aliases:  make(map[string]string),
		synonyms: make(map[string]string),
	}
}

// LoadDBFile loads a read-only SQLite database file into the manager.
func (m *Manager) LoadDBFile(path string) error {
	db, err := OpenFileRO(path)
	if err != nil {
		return fmt.Errorf("open module db %q: %w", path, err)
	}

	return m.AddDB(path, db)
}

// AddDB registers an already-opened database into the manager.
func (m *Manager) AddDB(path string, db *sql.DB) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Read product IDs from this database
	rows, err := db.Query(`SELECT id FROM products`)
	if err != nil {
		return fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var productIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		productIDs = append(productIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(productIDs) == 0 {
		// Fallback: derive from filename
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		productIDs = []string{base}
	}

	primaryID := productIDs[0]

	// 2. Read aliases
	aliasRows, err := db.Query(`SELECT alias, product_id FROM product_aliases`)
	var aliases []string
	if err == nil {
		defer aliasRows.Close()
		for aliasRows.Next() {
			var alias, prodID string
			if err := aliasRows.Scan(&alias, &prodID); err == nil {
				m.aliases[alias] = prodID
				aliases = append(aliases, alias)
			}
		}
	}

	// 3. Read synonyms
	synRows, err := db.Query(`SELECT term, expansion FROM synonyms`)
	if err == nil {
		defer synRows.Close()
		for synRows.Next() {
			var term, exp string
			if err := synRows.Scan(&term, &exp); err == nil {
				// Combine or override synonyms
				m.synonyms[term] = exp
			}
		}
	}

	mod := &ModuleDB{
		Path:      path,
		DB:        db,
		ProductID: primaryID,
		Aliases:   aliases,
	}

	for _, pid := range productIDs {
		m.modules[pid] = mod
	}

	return nil
}

// Close closes all loaded modular databases.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	closed := make(map[*sql.DB]bool)
	for _, mod := range m.modules {
		if !closed[mod.DB] {
			if err := mod.DB.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			closed[mod.DB] = true
		}
	}
	m.modules = make(map[string]*ModuleDB)
	m.aliases = make(map[string]string)
	m.synonyms = make(map[string]string)
	return firstErr
}

// ModuleCount returns number of loaded modules.
func (m *Manager) ModuleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[*sql.DB]bool)
	count := 0
	for _, mod := range m.modules {
		if !seen[mod.DB] {
			seen[mod.DB] = true
			count++
		}
	}
	return count
}

// LoadedProducts returns list of canonical product IDs currently loaded.
func (m *Manager) LoadedProducts() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var prods []string
	for pid := range m.modules {
		prods = append(prods, pid)
	}
	return prods
}

// Synonyms returns a copy of merged synonyms.
func (m *Manager) Synonyms() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]string, len(m.synonyms))
	for k, v := range m.synonyms {
		res[k] = v
	}
	return res
}

// ResolveProduct maps alias -> canonical product ID among loaded modules.
func (m *Manager) ResolveProduct(slug string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	slugLower := strings.ToLower(slug)
	if _, ok := m.modules[slugLower]; ok {
		return slugLower, nil
	}
	if canon, ok := m.aliases[slugLower]; ok {
		if _, exists := m.modules[canon]; exists {
			return canon, nil
		}
	}
	return "", fmt.Errorf("unknown product %q (loaded: %s)", slug, strings.Join(m.loadedProductListLocked(), ", "))
}

func (m *Manager) loadedProductListLocked() []string {
	var list []string
	for k := range m.modules {
		list = append(list, k)
	}
	if len(list) == 0 {
		return []string{"none"}
	}
	return list
}

// GetProduct retrieves product details from the appropriate module DB.
func (m *Manager) GetProduct(id string) (*Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	canon, ok := m.modules[id]
	if !ok {
		if a, okAlias := m.aliases[id]; okAlias {
			canon = m.modules[a]
		}
	}
	if canon == nil {
		return nil, fmt.Errorf("product %q not loaded or not found", id)
	}

	return GetProduct(canon.DB, canon.ProductID)
}

// SearchEndpoints searches across all loaded modules or a specific product.
func (m *Manager) SearchEndpoints(ftsQuery, productID, releasePrefix string, limit int) ([]SearchResult, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > 50 {
		limit = 10
	}

	var targetModules []*ModuleDB
	seenDBs := make(map[*sql.DB]bool)

	if productID != "" {
		canonID := productID
		if a, ok := m.aliases[productID]; ok {
			canonID = a
		}
		if mod, ok := m.modules[canonID]; ok {
			targetModules = append(targetModules, mod)
		} else {
			return nil, 0, fmt.Errorf("product %q not loaded", productID)
		}
	} else {
		for _, mod := range m.modules {
			if !seenDBs[mod.DB] {
				seenDBs[mod.DB] = true
				targetModules = append(targetModules, mod)
			}
		}
	}

	var allResults []SearchResult
	totalMatches := 0

	for _, mod := range targetModules {
		filterPID := ""
		if productID != "" {
			filterPID = mod.ProductID
		}
		res, count, err := SearchEndpoints(mod.DB, ftsQuery, filterPID, releasePrefix, limit)
		if err != nil {
			return nil, 0, err
		}
		allResults = append(allResults, res...)
		totalMatches += count
	}

	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, totalMatches, nil
}

// GetEndpoint retrieves full endpoint detail from the relevant module.
func (m *Manager) GetEndpoint(productID, releasePrefix, method, path string) (*Endpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	canonID := productID
	if a, ok := m.aliases[productID]; ok {
		canonID = a
	}
	mod, ok := m.modules[canonID]
	if !ok {
		return nil, fmt.Errorf("product %q not loaded", productID)
	}

	return GetEndpoint(mod.DB, mod.ProductID, releasePrefix, method, path)
}
