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

package modules

import (
	"encoding/json"
	"fmt"
	"os"
)

// ModuleInfo describes a single downloadable API module.
type ModuleInfo struct {
	Name        string   `json:"name"`
	ProductID   string   `json:"product_id"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	SizeBytes   int64    `json:"size_bytes,omitempty"`
	SHA256      string   `json:"sha256"`
	URL         string   `json:"url"`
	Aliases     []string `json:"aliases,omitempty"`
}

// Manifest contains the catalog of available API modules.
type Manifest struct {
	Version int                   `json:"version"`
	Modules map[string]ModuleInfo `json:"modules"`
}

// LoadManifest parses a JSON manifest from raw bytes.
func LoadManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest json: %w", err)
	}
	if m.Modules == nil {
		m.Modules = make(map[string]ModuleInfo)
	}
	return &m, nil
}

// SaveManifest writes the manifest to a file formatted with indentation.
func (m *Manifest) Save(path string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}
