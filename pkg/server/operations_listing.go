package server

// listItem is the unified shape returned by every datasource and network
// listing operation. Type-specific fields live under extra.
type listItem struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	URL         string         `json:"url"`
	Type        string         `json:"type"`
	Extra       map[string]any `json:"extra,omitempty"`
}
