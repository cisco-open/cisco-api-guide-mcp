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
