package domain

import "errors"

var (
	ErrClaimRefused   = errors.New("claim device target")
	ErrBindingRefused = errors.New("bind device command")
)

func (b CommandBinding) Claim() TargetClaim {
	return TargetClaim{Target: b.Target, Device: b.Device, Owner: b.Owner}
}
