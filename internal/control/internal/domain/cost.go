package domain

import (
	"errors"
	"fmt"
	"math"
)

// ErrCostReservationRejected means an aggregate cost limit or kill switch
// denied a new episode reservation. It is an expected admission outcome, not a
// storage failure.
var ErrCostReservationRejected = errors.New("cost ceiling or kill switch rejected")

// GlobalScope is the scope key of the global cost limit.
const GlobalScope = "global"

// TenantScope is the scope key of one tenant's cost limit.
func TenantScope(tenantID string) string { return "tenant:" + tenantID }

// Reservation statuses.
const (
	ReservationReserved = "reserved"
	ReservationSettled  = "settled"
)

// CheckReservation validates a reservation request. A zero amount is an
// unestimated reservation.
func CheckReservation(episodeID, tenantID string, amount uint64, now string) error {
	if episodeID == "" || tenantID == "" || now == "" || amount > math.MaxInt64 {
		return fmt.Errorf("invalid cost reservation")
	}
	return nil
}

// CheckSettlement validates a settlement request.
func CheckSettlement(episodeID string, actual uint64, now string) error {
	if episodeID == "" || now == "" || actual > math.MaxInt64 {
		return fmt.Errorf("invalid cost settlement")
	}
	return nil
}

// CheckLimit validates a cost limit configuration.
func CheckLimit(scopeKey string, maxMicro uint64, now string) error {
	if scopeKey == "" || now == "" || maxMicro > math.MaxInt64 {
		return fmt.Errorf("invalid cost limit")
	}
	return nil
}

// Micro converts a validated amount to the storage integer. The public checks
// above keep it within SQLite's signed range.
//
//nolint:gosec // range validation is performed by the Check functions.
func Micro(value uint64) int64 { return int64(value) }

// CheckNoActiveCeiling rejects an unestimated (zero) reservation while the
// scope's ceiling is set or its kill switch is engaged.
func CheckNoActiveCeiling(maxMicro int64, killSwitch bool) error {
	if maxMicro > 0 || killSwitch {
		return fmt.Errorf("%w: cost estimate is required while aggregate cost control is active", ErrCostReservationRejected)
	}
	return nil
}

// CheckReserved rejects a reservation that no limit row admitted.
func CheckReserved(rows int64, scopeKey string) error {
	if rows != 1 {
		return fmt.Errorf("%w: %s", ErrCostReservationRejected, scopeKey)
	}
	return nil
}

// RefuseMissingLimit decides what an absent limit row means: the global limit
// is required, a tenant limit is optional.
func RefuseMissingLimit(scopeKey string) error {
	if scopeKey == GlobalScope {
		return fmt.Errorf("global cost limit is missing")
	}
	return nil
}

// CheckLimitWritten requires a limit write to have changed exactly one row.
func CheckLimitWritten(rows int64) error {
	if rows != 1 {
		return fmt.Errorf("cost limit was not updated")
	}
	return nil
}

// CheckLimitSettled requires a limit settlement to have changed exactly one row.
func CheckLimitSettled(rows int64, scopeKey string) error {
	if rows != 1 {
		return fmt.Errorf("cost limit %s is missing", scopeKey)
	}
	return nil
}

// Reservation is a stored episode cost reservation.
type Reservation struct {
	TenantID string
	Reserved int64
	Actual   int64
	Status   string
}

// NeedsSettlement reports whether the reservation still has to be settled. A
// settled reservation accepts only a repeat of its recorded actual cost.
func (r Reservation) NeedsSettlement(episodeID string, actual uint64) (bool, error) {
	if r.Status == ReservationReserved {
		return true, nil
	}
	if r.Status == ReservationSettled && r.Actual == Micro(actual) {
		return false, nil
	}
	return false, fmt.Errorf("cost reservation %s was already settled with a different value", episodeID)
}

// CheckStoredLimit converts a stored limit to its unsigned form.
func CheckStoredLimit(scopeKey string, maxMicro int64) (uint64, error) {
	if maxMicro < 0 {
		return 0, fmt.Errorf("cost limit %s is negative", scopeKey)
	}
	return uint64(maxMicro), nil
}
