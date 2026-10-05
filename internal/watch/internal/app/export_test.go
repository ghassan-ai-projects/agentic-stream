package app

import "context"

// Fire exposes the single-watch fire use case to the behavior tests;
// production reaches it only through FireEvent.
func (s *Service) Fire(ctx context.Context, watchID, eventID, situationID, target string, features map[string]any) (bool, error) {
	return s.fire(ctx, watchID, eventID, situationID, target, features)
}

// AwaitRetry exposes the retry wait to the cancellation test.
func AwaitRetry(ctx context.Context) error { return awaitRetry(ctx) }
