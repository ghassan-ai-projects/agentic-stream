package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// SimulatorTrace validates the record grammar of one simulator trace:
// runtime_config first, then events and model activations in strictly
// increasing recorded time, then trace_end last.
type SimulatorTrace struct {
	options      SimulatorOptions
	line         int
	records      int
	configured   bool
	ended        bool
	lastRecorded time.Time
}

// NewSimulatorTrace starts a trace with the normalized options.
func NewSimulatorTrace(options SimulatorOptions) *SimulatorTrace {
	return &SimulatorTrace{options: options.Normalized()}
}

// Options returns the normalized options the trace converts events with.
func (t *SimulatorTrace) Options() SimulatorOptions { return t.options }

// NextLine counts one more trace line, blank or not.
func (t *SimulatorTrace) NextLine() { t.line++ }

// Line is the number of trace lines counted so far.
func (t *SimulatorTrace) Line() int { return t.line }

// Accept validates one non-blank record. For an event record it returns the
// converted envelope and true; control and activation records return false.
func (t *SimulatorTrace) Accept(raw []byte) (contractsv1.Envelope, bool, error) {
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		return contractsv1.Envelope{}, false, fmt.Errorf("parse simulator line %d: %w", t.line, err)
	}
	recordType, _ := record["record_type"].(string)
	if err := t.checkPosition(recordType); err != nil {
		return contractsv1.Envelope{}, false, err
	}
	return t.acceptRecord(recordType, record)
}

// Finish requires the trace to have opened with runtime_config and closed with
// trace_end.
func (t *SimulatorTrace) Finish() error {
	if !t.configured || !t.ended {
		return errors.New("simulator trace requires runtime_config first and trace_end last")
	}
	return nil
}

// checkPosition requires runtime_config first and nothing after trace_end.
func (t *SimulatorTrace) checkPosition(recordType string) error {
	t.records++
	if t.records == 1 && recordType != "runtime_config" {
		return fmt.Errorf("validate simulator line %d: runtime_config must be first", t.line)
	}
	if t.ended {
		return fmt.Errorf("validate simulator line %d: record follows trace_end", t.line)
	}
	return nil
}

func (t *SimulatorTrace) acceptRecord(recordType string, record map[string]any) (contractsv1.Envelope, bool, error) {
	switch recordType {
	case "runtime_config":
		return contractsv1.Envelope{}, false, t.acceptConfig(record)
	case "event":
		env, err := t.acceptEvent(record)
		return env, err == nil, err
	case "model_activation":
		return contractsv1.Envelope{}, false, t.acceptModelActivation(record)
	case "trace_end":
		return contractsv1.Envelope{}, false, t.acceptTraceEnd(record)
	default:
		return contractsv1.Envelope{}, false, fmt.Errorf("unknown simulator record_type %q", recordType)
	}
}

func (t *SimulatorTrace) acceptModelActivation(record map[string]any) error {
	recorded, err := validateModelActivation(record)
	if err != nil {
		return fmt.Errorf("validate simulator line %d: %w", t.line, err)
	}
	if !t.after(recorded) {
		return errors.New("recorded time must be strictly increasing")
	}
	t.lastRecorded = recorded
	return nil
}

func (t *SimulatorTrace) acceptTraceEnd(record map[string]any) error {
	if err := validateSimulatorControl("trace_end", record); err != nil {
		return fmt.Errorf("validate simulator line %d: %w", t.line, err)
	}
	until, _ := parseSimulatorTime(record, "until")
	if !t.after(until) {
		return errors.New("trace_end until must be later than every recorded input")
	}
	t.ended = true
	return nil
}

func (t *SimulatorTrace) acceptConfig(record map[string]any) error {
	if t.configured {
		return fmt.Errorf("validate simulator line %d: duplicate runtime_config", t.line)
	}
	if err := validateSimulatorControl("runtime_config", record); err != nil {
		return fmt.Errorf("validate simulator line %d: %w", t.line, err)
	}
	t.configured = true
	return nil
}

func (t *SimulatorTrace) acceptEvent(record map[string]any) (contractsv1.Envelope, error) {
	env, err := t.options.ConvertEvent(record)
	if err != nil {
		return contractsv1.Envelope{}, fmt.Errorf("convert simulator line %d: %w", t.line, err)
	}
	if err := t.advanceRecorded(env.ObservedAt); err != nil {
		return contractsv1.Envelope{}, err
	}
	return env, nil
}

// advanceRecorded requires the event's arrival time to move recorded time
// strictly forward.
func (t *SimulatorTrace) advanceRecorded(recorded *time.Time) error {
	if recorded == nil {
		return errors.New("event arrival_time is required")
	}
	if !t.after(*recorded) {
		return errors.New("event recorded time must be strictly increasing")
	}
	t.lastRecorded = recorded.UTC()
	return nil
}

// after reports whether recorded is later than every input seen so far.
func (t *SimulatorTrace) after(recorded time.Time) bool {
	return t.lastRecorded.IsZero() || recorded.After(t.lastRecorded)
}
