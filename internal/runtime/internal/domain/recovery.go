package domain

// EpisodeRecoveryReport describes attempts and episodes repaired before readiness.
type EpisodeRecoveryReport struct{ AbandonedAttempts, RequeuedEpisodes, AbandonedEpisodes int }

// RecoveryReport combines durable episode and evidence repairs.
type RecoveryReport struct {
	Episodes            EpisodeRecoveryReport
	InterruptedEvidence int
}
