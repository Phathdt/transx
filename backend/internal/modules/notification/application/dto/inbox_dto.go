package dto

// InboxItemResponse is one user-facing inbox message returned to clients.
// transferId is the business reference (ITN-/ETN-…), matching transfer API.
type InboxItemResponse struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	TransferID string  `json:"transferId,omitempty"`
	ReadAt     *string `json:"readAt,omitempty"`
	CreatedAt  string  `json:"createdAt"`
}

// ListInboxQuery documents the GET /inbox query params for the OpenAPI spec.
// Fields carry only the query tag (no json) so they render as query params,
// not a request body.
type ListInboxQuery struct {
	Page     int `query:"page"`
	PageSize int `query:"pageSize"`
}

// InboxListResponse is the paginated inbox for the caller.
type InboxListResponse struct {
	Data     []InboxItemResponse `json:"data"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
	Total    int64               `json:"total"`
}

// UnreadCountResponse is the unread inbox item count.
type UnreadCountResponse struct {
	Count int64 `json:"count"`
}

// ReadAllResponse reports how many unread items were marked read.
type ReadAllResponse struct {
	Updated int64 `json:"updated"`
}
