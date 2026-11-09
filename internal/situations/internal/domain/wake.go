package domain

import (
	"fmt"
	"time"
)

func (e *Engine) Woken(situationID string, watermark time.Time) (*Version, bool, error) {
	sit, found := e.openSituation(situationID)
	if !found {
		return nil, false, nil
	}
	version, err := e.publish(sit, sit.Phase, watermark)
	if err != nil {
		return nil, false, fmt.Errorf("publish woken situation %s: %w", situationID, err)
	}
	version.WatchFired = true
	return version, true, nil
}

func (e *Engine) PartitionOf(situationID string) (int, bool) {
	sit, found := e.openSituation(situationID)
	if !found {
		return 0, false
	}
	return sit.PartitionID, true
}

func (e *Engine) openSituation(situationID string) (*Situation, bool) {
	for _, sit := range e.active {
		if sit.SituationID == situationID && sit.Version > 0 && sit.Phase != PhaseResolved {
			return sit, true
		}
	}
	return nil, false
}
