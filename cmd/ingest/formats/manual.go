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
