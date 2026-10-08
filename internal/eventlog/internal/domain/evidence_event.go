package domain

// EvidenceEvent locates one logged event an explanation cites: its log
// position, identity, source and times.
type EvidenceEvent struct {
	Position   int64  `json:"position"`
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	Source     string `json:"source"`
	EntityID   string `json:"entity_id"`
	EventTime  string `json:"event_time"`
	IngestedAt string `json:"ingested_at"`
}
