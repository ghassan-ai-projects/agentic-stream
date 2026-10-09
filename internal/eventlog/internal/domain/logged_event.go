package domain

import "bytes"

const ReasonEventIDConflict = "event_id_conflict"

type LoggedEvent struct {
	EventType, EntityType, EntityID, EventTime string
	PayloadSHA256                              []byte
}

func (l LoggedEvent) SameEvent(other LoggedEvent) bool {
	return l.EventType == other.EventType && l.EntityType == other.EntityType && l.EntityID == other.EntityID &&
		l.EventTime == other.EventTime && bytes.Equal(l.PayloadSHA256, other.PayloadSHA256)
}
