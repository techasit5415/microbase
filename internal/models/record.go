package models

// Record represents a single dynamic row in a user-defined collection.
type Record map[string]any

// ListOptions holds pagination, sorting, search, and exact-match filter parameters.
type ListOptions struct {
	Page    int
	PerPage int
	Sort    string            // Example: "-created_at" (DESC) or "title" (ASC)
	Search  string            // Full-text search term across text columns
	Filters    map[string]string
	FilterExpr string
	SkipTotal  bool
	AuthClaims any // Exact-match filters, e.g. ?filter_published=true
}

// Normalize enforces safe defaults and upper bounds on pagination options.
func (o *ListOptions) Normalize() {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.PerPage < 1 {
		o.PerPage = 20
	}
	if o.PerPage > 100 {
		o.PerPage = 100
	}
}

// Offset returns the SQL OFFSET derived from Page and PerPage.
func (o ListOptions) Offset() int {
	return (o.Page - 1) * o.PerPage
}

// ListResult wraps paginated collection records with metadata.
type ListResult struct {
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
	TotalItems int64    `json:"total_items"`
	Items      []Record `json:"items"`
}

// VectorSearchRequest is the payload accepted by POST /api/collections/:name/search/vector.
type VectorSearchRequest struct {
	Field  string    `json:"field"`  // Name of the vector column (e.g., "embedding")
	Vector []float64 `json:"vector"` // Query embedding vector
	Limit  int       `json:"limit"`  // Max results (default 10, max 100)
}
