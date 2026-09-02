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

package formats

import (
	"fmt"

	idb "github.com/cisco-open/cisco-api-guide-mcp/internal/db"
)

// FormatHandler parses raw API doc bytes into endpoints.
type FormatHandler interface {
	Parse(productID string, data []byte) ([]idb.Endpoint, error)
}

// Handlers is the registry of all supported format handlers.
var Handlers = map[string]FormatHandler{
	"openapi3": &OpenAPI3Handler{},
	"aci-meta": &ACIMetaHandler{},
	"swagger2": &Swagger2Handler{},
	"manual":   &ManualHandler{},
}

// Swagger2Handler parses Swagger 2.0 documents.
type Swagger2Handler struct{}

func (h *Swagger2Handler) Parse(productID string, data []byte) ([]idb.Endpoint, error) {
	return nil, fmt.Errorf("format %q: not implemented", "swagger2")
}
