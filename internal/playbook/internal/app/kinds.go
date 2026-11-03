package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
)

func (s *Service) knownKinds(ctx context.Context, tx *store.Tx, now time.Time) (map[string]string, error) {
	if s.kinds != nil {
		if err := tx.RecordKindDigests(ctx, s.tenantID, s.kinds(), now); err != nil {
			return nil, err
		}
	}
	return tx.KindDigests(ctx, s.tenantID)
}

func (s *Service) entityKind(entityID string) string {
	if s.entityKinds == nil {
		return ""
	}
	return s.entityKinds(entityID)
}

func kindNamed(kinds map[string]string, digest string) string {
	for name, born := range kinds {
		if born == digest {
			return name
		}
	}
	return ""
}
