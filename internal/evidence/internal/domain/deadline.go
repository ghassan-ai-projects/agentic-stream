package domain

import (
	"time"
)

// CallDeadline caps worker time at capability expiry.
func CallDeadline(req Envelope, scope Scope, now time.Time) (time.Time, error) {
	deadline := now.Add(time.Minute)
	if req.Deadline.Present {
		if !req.Deadline.Valid {
			return time.Time{}, Refuse(InvalidArgument, "deadline is invalid")
		}
		deadline = req.Deadline.Value
	}
	if !deadline.Before(scope.ExpiresAt) {
		deadline = scope.ExpiresAt
	}
	if !deadline.After(now) {
		return time.Time{}, Refuse(DeadlineExceeded, "evidence call deadline has expired")
	}
	return deadline, nil
}
