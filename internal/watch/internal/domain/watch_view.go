package domain

// WatchView is one watch an approved command installed, as an operator
// inspects it: what it watches, its allowance and every time it fired.
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

// FireView is one evidence event that fired a watch.
type FireView struct {
	EventID string `json:"event_id"`
	FiredAt string `json:"fired_at"`
}
