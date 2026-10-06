package domain

import (
	"context"
	"errors"
)

// ErrorKind identifies a stable tool refusal category.
type ErrorKind string

const (
	InvalidArgument    ErrorKind = "invalid_argument"
	ResourceExhausted  ErrorKind = "resource_exhausted"
	PermissionDenied   ErrorKind = "permission_denied"
	FailedPrecondition ErrorKind = "failed_precondition"
	AlreadyExists      ErrorKind = "already_exists"
	Internal           ErrorKind = "internal"
	Canceled           ErrorKind = "canceled"
	DeadlineExceeded   ErrorKind = "deadline_exceeded"
)

// Refusal carries safe public text without leaking provider details.
type Refusal struct {
	Kind    ErrorKind
	Message string
}

func (e *Refusal) Error() string { return e.Message }

// Refuse constructs one typed denial.
func Refuse(kind ErrorKind, message string) error { return &Refusal{Kind: kind, Message: message} }

// ContextRefusal classifies cancellation without transport dependencies.
func ContextRefusal(err error) error {
	if errors.Is(err, context.Canceled) {
		return Refuse(Canceled, "evidence query canceled")
	}
	return Refuse(DeadlineExceeded, "evidence query deadline exceeded")
}

// ReservationRefusal rejects terminal or already-running call identities.
func ReservationRefusal(reservation Reservation) error {
	if reservation.Created {
		return nil
	}
	if reservation.Status != "running" {
		return Refuse(FailedPrecondition, "evidence call is terminal")
	}
	return Refuse(AlreadyExists, "evidence call is already in progress")
}

// ResultViolation returns the stable persisted failure code before querying again.
func ResultViolation(call Call, result QueryResult) (string, error) {
	if uint64(len(result.JSON)) > call.MaxBytes {
		return "result_bytes_exceeded", Refuse(ResourceExhausted, "evidence result exceeds capability")
	}
	if result.RowCount > call.MaxRows {
		return "result_rows_exceeded", Refuse(ResourceExhausted, "evidence result exceeds row limit")
	}
	return "", nil
}
