package app

import "context"

func (s *Service) Fire(ctx context.Context, watchID, eventID, situationID, target string, features map[string]any) (bool, error) {
	return s.fire(ctx, watchID, eventID, situationID, target, features)
}
