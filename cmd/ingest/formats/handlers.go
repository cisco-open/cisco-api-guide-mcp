package formats

import (
	"fmt"

	idb "github.com/brightpuddle/cisco-api-guide-mcp/internal/db"
)

// FormatHandler parses raw API doc bytes into endpoints.
type FormatHandler interface {
	Parse(productID string, data []byte) ([]idb.Endpoint, error)
}

var Handlers = map[string]FormatHandler{
	"openapi3": &OpenAPI3Handler{},
	"swagger2": &Swagger2Handler{},
	"manual":   &ManualHandler{},
}

// OpenAPI3Handler parses OpenAPI 3.x documents.
type OpenAPI3Handler struct{}

func (h *OpenAPI3Handler) Parse(productID string, data []byte) ([]idb.Endpoint, error) {
	return nil, fmt.Errorf("format %q: not implemented", "openapi3")
}

// Swagger2Handler parses Swagger 2.0 documents.
type Swagger2Handler struct{}

func (h *Swagger2Handler) Parse(productID string, data []byte) ([]idb.Endpoint, error) {
	return nil, fmt.Errorf("format %q: not implemented", "swagger2")
}
