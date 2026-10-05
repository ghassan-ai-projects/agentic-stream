// Package policy owns the deterministic authorization boundary between
// accepted Intents and Commands.
package policy

import (
	"fmt"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

// Gateway evaluates accepted Intents against current durable state.
type Gateway struct {
	policyVersion string
	policyDigest  string
	idGen         ids.Generator
	owner         *runtimecontrol.RuntimeOwner
	ownerEpoch    string
	interlock     interlock.Reader
	// P8: automatic consequential intents require an exact calibration artifact.
	calibration *qualification.CalibrationStore
	// P8: decisions admitted under a killed policy epoch are refused here.
	epochControl *runtimecontrol.EpochControl
}

// WithCalibration enables the calibration gate for automatic consequential intents.
func (g *Gateway) WithCalibration(store *qualification.CalibrationStore) *Gateway {
	g.calibration = store
	return g
}

// WithEpochControl enables the kill gate at the governance boundary.
func (g *Gateway) WithEpochControl(control *runtimecontrol.EpochControl) *Gateway {
	g.epochControl = control
	return g
}

// NewGateway creates a deterministic policy gateway.
func NewGateway(policyVersion string, idGen ids.Generator) *Gateway {
	return newGateway(policyVersion, idGen, nil, "")
}

// NewGatewayWithOwner creates a policy gateway that fences every mutation to
// the active runtime lease.
func NewGatewayWithOwner(policyVersion string, idGen ids.Generator, owner *runtimecontrol.RuntimeOwner, ownerEpoch string) *Gateway {
	return newGateway(policyVersion, idGen, owner, ownerEpoch)
}

// WithInterlock adds the durable read-only action readiness check.
func (g *Gateway) WithInterlock(reader interlock.Reader) *Gateway {
	g.interlock = reader
	return g
}

func newGateway(policyVersion string, idGen ids.Generator, owner *runtimecontrol.RuntimeOwner, ownerEpoch string) *Gateway {
	if idGen == nil {
		idGen = ids.Random()
	}
	policyDigest, err := DigestForVersion(policyVersion)
	if err != nil {
		panic(fmt.Sprintf("construct policy gateway: %v", err))
	}
	return &Gateway{policyVersion: policyVersion, policyDigest: policyDigest, idGen: idGen, owner: owner, ownerEpoch: ownerEpoch}
}
