package domain

type ApprovalView struct {
	ApprovalID       string `json:"approval_id"`
	Status           string `json:"status"`
	RequestedAt      string `json:"requested_at"`
	ExpiresAt        string `json:"expires_at"`
	DecidedAt        string `json:"decided_at,omitempty"`
	Approver         string `json:"approver,omitempty"`
	Relay            string `json:"relay,omitempty"`
	Reason           string `json:"reason,omitempty"`
	WithdrawnAt      string `json:"withdrawn_at,omitempty"`
	WithdrawalReason string `json:"withdrawal_reason,omitempty"`
}
