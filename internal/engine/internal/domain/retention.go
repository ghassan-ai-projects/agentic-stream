package domain

type PruneReport struct {
	InboxEntries, Timers, SituationVersions, LineageSets int64
}
