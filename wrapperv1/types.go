package wrapperv1

// SearchRequest is the payload for POST /search.
type SearchRequest struct {
	Query   string         `json:"query"`
	Filters map[string]any `json:"filters,omitempty"`
	Page    *Page          `json:"page,omitempty"`
}

// Page defines pagination details.
type Page struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Item is a normalized result item.
type Item struct {
	ID        string         `json:"id"`
	Title     string         `json:"title,omitempty"`
	Type      string         `json:"type,omitempty"`
	Snippet   string         `json:"snippet,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	SourceURL string         `json:"source_url,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// SearchResponse is returned from POST /search.
type SearchResponse struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// GetRequest is the payload for POST /get.
type GetRequest struct {
	ID string `json:"id"`
}

// GetResponse is returned from POST /get.
type GetResponse struct {
	ID        string         `json:"id"`
	Title     string         `json:"title,omitempty"`
	Type      string         `json:"type,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	SizeBytes *int64         `json:"size_bytes,omitempty"`
	MimeType  string         `json:"mime_type,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ContentRequest is the payload for POST /content.
type ContentRequest struct {
	ID       string `json:"id"`
	Format   string `json:"format,omitempty"`
	MaxBytes *int64 `json:"max_bytes,omitempty"`
}

// ContentResponse is returned from POST /content.
type ContentResponse struct {
	ID        string `json:"id"`
	Format    string `json:"format,omitempty"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ErrorResponse is returned on adapter errors.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody defines the error payload.
type ErrorBody struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	RetryAfterMS *int64 `json:"retry_after_ms,omitempty"`
}
