package domain

type WatchView struct {
	WatchID          string     `json:"watch_id"`
	SituationID      string     `json:"situation_id"`
	SituationVersion int        `json:"situation_version"`
	Expression       string     `json:"expression"`
	Target           string     `json:"target"`
	Status           string     `json:"status"`
	ExpiresAt        string     `json:"expires_at"`
	RemainingFires   int        `json:"remaining_fires"`
	MaxFires         int        `json:"max_fires"`
	Fires            []FireView `json:"fires"`
}

type FireView struct {
	EventID string `json:"event_id"`
	FiredAt string `json:"fired_at"`
}
