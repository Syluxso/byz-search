package main

// Search API + analytics shapes (no DB).

type problem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
}

type TokenClaims struct {
	OrganizationID string
	TenantID       string
	UserID         string
	ClientID       string
	Subject        string
}

type SearchHit struct {
	ID             string   `json:"id"`
	Title          string   `json:"title,omitempty"`
	Snippet        string   `json:"snippet,omitempty"`
	Score          float64  `json:"score"`
	Source         string   `json:"source,omitempty"`
	Path           string   `json:"path,omitempty"`
	OrganizationID string   `json:"organizationId,omitempty"`
	TenantID       string   `json:"tenantId,omitempty"`
	UserID         string   `json:"userId,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

type SearchResponse struct {
	Q          string      `json:"q"`
	Page       int         `json:"page"`
	Size       int         `json:"size"`
	Total      int64       `json:"total"`
	DurationMs int64       `json:"durationMs"`
	Hits       []SearchHit `json:"hits"`
}

// DocumentResponse is the full indexed body for one Solr doc (agent get_document).
type DocumentResponse struct {
	ID             string   `json:"id"`
	Title          string   `json:"title,omitempty"`
	Source         string   `json:"source,omitempty"`
	Path           string   `json:"path,omitempty"`
	OrganizationID string   `json:"organizationId,omitempty"`
	TenantID       string   `json:"tenantId,omitempty"`
	UserID         string   `json:"userId,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Content        string   `json:"content"`
	ContentChars   int      `json:"contentChars"`
	Truncated      bool     `json:"truncated"`
	MaxChars       int      `json:"maxChars"`
}

// SearchQueryEvent is emitted on byz.search.query after each search.
type SearchQueryEvent struct {
	EventID        string `json:"eventId"`
	Type           string `json:"type"`
	OccurredAt     string `json:"occurredAt"`
	RequestID      string `json:"requestId"`
	OrganizationID string `json:"organizationId"`
	TenantID       string `json:"tenantId,omitempty"`
	UserID         string `json:"userId,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	Q              string `json:"q"`
	Page           int    `json:"page"`
	Size           int    `json:"size"`
	Total          int64  `json:"total"`
	HitCount       int    `json:"hitCount"`
	DurationMs     int64  `json:"durationMs"`
	Status         int    `json:"status"`
}
