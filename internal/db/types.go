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

// Product represents one Cisco API product.
type Product struct {
	ID          string
	Name        string
	Description string
	BaseURL     string
	AuthType    string
	AuthNotes   string
	AuthSchema  string // raw JSON
}

// Endpoint represents one HTTP operation.
type Endpoint struct {
	ID           int64
	ProductID    string
	Method       string
	Path         string
	Summary      string
	Description  string
	Tags         string // raw JSON array
	Parameters   string // raw JSON array
	RequestBody  string // raw JSON object
	Responses    string // raw JSON object
	SourceFormat string
}

// SearchResult is a lightweight endpoint for search output.
type SearchResult struct {
	ProductID string
	Method    string
	Path      string
	Summary   string
}
