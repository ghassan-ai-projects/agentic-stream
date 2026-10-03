package ingress

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

//go:embed simulator_data.json
var simulatorData []byte

// The channel→field mapping (simulator_data.json) — domain DATA, not code.
// An empty target is the heartbeat/no-data sentinel; a missing channel falls
// back to data["value"].
var channelFields = sync.OnceValues(func() (map[string]string, error) {
	var document struct {
		ChannelFields map[string]string `json:"channel_fields"`
	}
	if err := json.Unmarshal(simulatorData, &document); err != nil {
		return nil, fmt.Errorf("decode simulator data: %w", err)
	}
	return document.ChannelFields, nil
})

// SimulatorOptions describes the consumer-specific projection from the
// streams-simulator trace-record-v0.1 format into current normalized events.
type SimulatorOptions struct {
	TenantID        string
	EntityType      string
	EventTypePrefix string
	Source          string
}

// SimulatorJSONLReplay consumes the streams-simulator Agentic Stream adapter
// format. It deliberately does not treat headers or trailers as events.
type SimulatorJSONLReplay struct {
	db          *storage.DB
	log         *eventlog.EventLog
	options     SimulatorOptions
	path        string
	connectorID string
	clk         clock.Clock
}

// NewSimulatorJSONLReplay creates a strict simulator adapter reader.
func NewSimulatorJSONLReplay(db *storage.DB, log *eventlog.EventLog, options SimulatorOptions, path, connectorID string) *SimulatorJSONLReplay {
	if options.TenantID == "" {
		options.TenantID = "default"
	}
	if options.EventTypePrefix == "" && options.EntityType != "" {
		options.EventTypePrefix = options.EntityType + "."
	}
	if options.Source == "" {
		options.Source = "streams-simulator"
	}
	if connectorID == "" {
		connectorID = "simulator-jsonl:" + path
	}
	return &SimulatorJSONLReplay{db: db, log: log, options: options, path: path, connectorID: connectorID, clk: clock.Physical()}
}

// Run appends all new event records from the simulator trace.
func (r *SimulatorJSONLReplay) Run(ctx context.Context) (int, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return 0, fmt.Errorf("open simulator trace: %w", err)
	}
	defer func() { _ = f.Close() }()
	start, err := loadLineCheckpoint(ctx, r.db, r.connectorID)
	if err != nil {
		return 0, err
	}
	trace := simulatorTrace{replay: r, startLine: start, batch: envelopeBatch{log: r.log, tenantID: r.options.TenantID}}
	err = trace.run(ctx, f)
	return trace.batch.appended, err
}

// run reads the whole trace, requires its framing records, then appends the
// final batch and checkpoints the last line.
func (t *simulatorTrace) run(ctx context.Context, f io.Reader) error {
	if err := t.readLines(ctx, f); err != nil {
		return err
	}
	if !t.configured || !t.ended {
		return fmt.Errorf("simulator trace requires runtime_config first and trace_end last")
	}
	if err := t.batch.flush(ctx); err != nil {
		return fmt.Errorf("append simulator batch: %w", err)
	}
	return saveLineCheckpoint(ctx, t.replay.db, t.replay.connectorID, t.line, t.replay.clk.Now())
}

func (t *simulatorTrace) readLines(ctx context.Context, f io.Reader) error {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t.line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		if err := t.accept(ctx, scanner.Bytes()); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read simulator trace: %w", err)
	}
	return nil
}

// simulatorTrace validates the record grammar of one simulator trace:
// runtime_config first, then events and model activations in strictly
// increasing recorded time, then trace_end last. Events after the checkpoint
// are batched for the event log.
type simulatorTrace struct {
	replay       *SimulatorJSONLReplay
	startLine    int
	line         int
	records      int
	configured   bool
	ended        bool
	lastRecorded time.Time
	batch        envelopeBatch
}

func (t *simulatorTrace) accept(ctx context.Context, raw []byte) error {
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		return fmt.Errorf("parse simulator line %d: %w", t.line, err)
	}
	recordType, _ := record["record_type"].(string)
	if err := t.checkPosition(recordType); err != nil {
		return err
	}
	return t.acceptRecord(ctx, recordType, record)
}

// checkPosition requires runtime_config first and nothing after trace_end.
func (t *simulatorTrace) checkPosition(recordType string) error {
	t.records++
	if t.records == 1 && recordType != "runtime_config" {
		return fmt.Errorf("validate simulator line %d: runtime_config must be first", t.line)
	}
	if t.ended {
		return fmt.Errorf("validate simulator line %d: record follows trace_end", t.line)
	}
	return nil
}

func (t *simulatorTrace) acceptRecord(ctx context.Context, recordType string, record map[string]any) error {
	switch recordType {
	case "runtime_config":
		return t.acceptConfig(record)
	case "event":
		return t.acceptEvent(ctx, record)
	case "model_activation":
		return t.acceptModelActivation(record)
	case "trace_end":
		return t.acceptTraceEnd(record)
	default:
		return fmt.Errorf("unknown simulator record_type %q", recordType)
	}
}

func (t *simulatorTrace) acceptModelActivation(record map[string]any) error {
	recorded, err := validateModelActivation(record)
	if err != nil {
		return fmt.Errorf("validate simulator line %d: %w", t.line, err)
	}
	if !t.after(recorded) {
		return fmt.Errorf("recorded time must be strictly increasing")
	}
	t.lastRecorded = recorded
	return nil
}

func (t *simulatorTrace) acceptTraceEnd(record map[string]any) error {
	if err := validateSimulatorControl("trace_end", record); err != nil {
		return fmt.Errorf("validate simulator line %d: %w", t.line, err)
	}
	until, _ := parseSimulatorTime(record, "until")
	if !t.after(until) {
		return fmt.Errorf("trace_end until must be later than every recorded input")
	}
	t.ended = true
	return nil
}

func (t *simulatorTrace) acceptConfig(record map[string]any) error {
	if t.configured {
		return fmt.Errorf("validate simulator line %d: duplicate runtime_config", t.line)
	}
	if err := validateSimulatorControl("runtime_config", record); err != nil {
		return fmt.Errorf("validate simulator line %d: %w", t.line, err)
	}
	t.configured = true
	return nil
}

func (t *simulatorTrace) acceptEvent(ctx context.Context, record map[string]any) error {
	env, err := t.replay.convertEvent(record)
	if err != nil {
		return fmt.Errorf("convert simulator line %d: %w", t.line, err)
	}
	if err := t.advanceRecorded(env.ObservedAt); err != nil {
		return err
	}
	if t.line <= t.startLine {
		return nil
	}
	if err := t.batch.add(ctx, env); err != nil {
		return fmt.Errorf("append simulator batch: %w", err)
	}
	return nil
}

// advanceRecorded requires the event's arrival time to move recorded time
// strictly forward.
func (t *simulatorTrace) advanceRecorded(recorded *time.Time) error {
	if recorded == nil {
		return fmt.Errorf("event arrival_time is required")
	}
	if !t.after(*recorded) {
		return fmt.Errorf("event recorded time must be strictly increasing")
	}
	t.lastRecorded = recorded.UTC()
	return nil
}

// after reports whether recorded is later than every input seen so far.
func (t *simulatorTrace) after(recorded time.Time) bool {
	return t.lastRecorded.IsZero() || recorded.After(t.lastRecorded)
}
