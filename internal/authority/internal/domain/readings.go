package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const rebootReason = "device_rebooted"

type Reading struct {
	Output     string
	Value      any
	Good       bool
	Historical bool
	BootID     string
	ReceivedAt time.Time
}

type ReadingEvidence struct {
	Device   DeviceBoot
	Owner    Owner
	Source   string
	State    map[string]any
	Outputs  []string
	Readings []Reading
}

func (e ReadingEvidence) Complete() bool {
	return e.Device.Complete() && e.Owner.Complete() && e.Source != "" && len(e.State) > 0 && len(e.Outputs) > 0
}

type Opening struct {
	At     time.Time
	Reboot bool
}

func NewOpening(at time.Time, details []byte) (Opening, error) {
	var fields map[string]any
	if err := json.Unmarshal(details, &fields); err != nil {
		return Opening{}, fmt.Errorf("read the reconciliation opening: %w", err)
	}
	return Opening{At: at, Reboot: fields["reason"] == rebootReason}, nil
}

func (e ReadingEvidence) Resolution(opening Opening) (ResolutionRequest, bool, error) {
	readings, clears := e.clearing(opening)
	if !clears {
		return ResolutionRequest{}, false, nil
	}
	sealed, err := SealReconciliationEvidence(e.Source, e.Device.DeviceID, e.State, readingFeedback(e.Outputs, readings, opening))
	if err != nil {
		return ResolutionRequest{}, false, err
	}
	return ResolutionRequest{Device: e.Device, Owner: e.Owner, Outcome: ResolutionSucceeded, Evidence: sealed.Document()}, true, nil
}

func (e ReadingEvidence) clearing(opening Opening) ([]Reading, bool) {
	chosen := make([]Reading, 0, len(e.Outputs))
	for _, output := range e.Outputs {
		found := slices.IndexFunc(e.Readings, func(r Reading) bool { return r.Output == output && r.clears(e.Device, opening) })
		if found < 0 {
			return nil, false
		}
		chosen = append(chosen, e.Readings[found])
	}
	return chosen, true
}

func (r Reading) clears(device DeviceBoot, opening Opening) bool {
	return r.Good && !r.Historical && r.BootID == device.BootID && (opening.Reboot || r.ReceivedAt.After(opening.At))
}

func readingFeedback(outputs []string, readings []Reading, opening Opening) map[string]any {
	documents := make([]any, len(readings))
	for i, reading := range readings {
		documents[i] = map[string]any{"output": reading.Output, "value": reading.Value, "boot_id": reading.BootID,
			"received_at": kernel.FormatTime(reading.ReceivedAt)}
	}
	return map[string]any{"outputs": slices.Clone(outputs), "readings": documents,
		"opened_at": kernel.FormatTime(opening.At), "opened_by_reboot": opening.Reboot}
}
