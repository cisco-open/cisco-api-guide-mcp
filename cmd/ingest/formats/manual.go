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
	"encoding/json"
	"fmt"

	idb "github.com/brightpuddle/cisco-api-guide-mcp/internal/db"
)

// ManualHandler accepts JSON matching internal endpoint schema directly.
type ManualHandler struct{}

// manualInput is the JSON input format for manual ingestion.
type manualInput struct {
	Endpoints []struct {
		Method       string          `json:"method"`
		Path         string          `json:"path"`
		Summary      string          `json:"summary"`
		Description  string          `json:"description"`
		Tags         json.RawMessage `json:"tags"`
		Parameters   json.RawMessage `json:"parameters"`
		RequestBody  json.RawMessage `json:"request_body"`
		Responses    json.RawMessage `json:"responses"`
	} `json:"endpoints"`
}

func (h *ManualHandler) Parse(productID string, data []byte) ([]idb.Endpoint, error) {
	var input manualInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("manual parse: %w", err)
	}

	var endpoints []idb.Endpoint
	for _, e := range input.Endpoints {
		tags := string(e.Tags)
		if tags == "" || tags == "null" {
			tags = "[]"
		}
		params := string(e.Parameters)
		if params == "" || params == "null" {
			params = "[]"
		}
		rb := string(e.RequestBody)
		if rb == "" || rb == "null" {
			rb = "{}"
		}
		responses := string(e.Responses)
		if responses == "" || responses == "null" {
			responses = "{}"
		}

		endpoints = append(endpoints, idb.Endpoint{
			ProductID:    productID,
			Method:       e.Method,
			Path:         e.Path,
			Summary:      e.Summary,
			Description:  e.Description,
			Tags:         tags,
			Parameters:   params,
			RequestBody:  rb,
			Responses:    responses,
			SourceFormat: "manual",
		})
	}
	return endpoints, nil
}
