package ingress

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
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
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		trace.line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		if err := trace.accept(ctx, scanner.Bytes()); err != nil {
			return trace.batch.appended, err
		}
	}
	if err := scanner.Err(); err != nil {
		return trace.batch.appended, fmt.Errorf("read simulator trace: %w", err)
	}
	if !trace.configured || !trace.ended {
		return trace.batch.appended, fmt.Errorf("simulator trace requires runtime_config first and trace_end last")
	}
	if err := trace.batch.flush(ctx); err != nil {
		return trace.batch.appended, fmt.Errorf("append simulator batch: %w", err)
	}
	if err := saveLineCheckpoint(ctx, r.db, r.connectorID, trace.line, r.clk.Now()); err != nil {
		return trace.batch.appended, err
	}
	return trace.batch.appended, nil
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
	t.records++
	if t.records == 1 && recordType != "runtime_config" {
		return fmt.Errorf("validate simulator line %d: runtime_config must be first", t.line)
	}
	if t.ended {
		return fmt.Errorf("validate simulator line %d: record follows trace_end", t.line)
	}
	switch recordType {
	case "runtime_config":
		return t.acceptConfig(record)
	case "event":
		return t.acceptEvent(ctx, record)
	case "model_activation":
		recorded, err := validateModelActivation(record)
		if err != nil {
			return fmt.Errorf("validate simulator line %d: %w", t.line, err)
		}
		if !t.after(recorded) {
			return fmt.Errorf("recorded time must be strictly increasing")
		}
		t.lastRecorded = recorded
		return nil
	case "trace_end":
		if err := validateSimulatorControl(recordType, record); err != nil {
			return fmt.Errorf("validate simulator line %d: %w", t.line, err)
		}
		until, _ := parseSimulatorTime(record, "until")
		if !t.after(until) {
			return fmt.Errorf("trace_end until must be later than every recorded input")
		}
		t.ended = true
		return nil
	default:
		return fmt.Errorf("unknown simulator record_type %q", recordType)
	}
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
	recorded := env.ObservedAt
	if recorded == nil {
		return fmt.Errorf("event arrival_time is required")
	}
	if !t.after(*recorded) {
		return fmt.Errorf("event recorded time must be strictly increasing")
	}
	t.lastRecorded = recorded.UTC()
	if t.line <= t.startLine {
		return nil
	}
	if err := t.batch.add(ctx, env); err != nil {
		return fmt.Errorf("append simulator batch: %w", err)
	}
	return nil
}

// after reports whether recorded is later than every input seen so far.
func (t *simulatorTrace) after(recorded time.Time) bool {
	return t.lastRecorded.IsZero() || recorded.After(t.lastRecorded)
}

func (r *SimulatorJSONLReplay) convertEvent(record map[string]any) (contractsv1.Envelope, error) {
	event, err := simulatorEventFields(record)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	getString := func(key string) (string, error) {
		value, ok := event[key].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return value, nil
	}
	id, err := getString("id")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	entityID, err := getString("entity_id")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	entityType, err := getString("entity_type")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	if r.options.EntityType != "" && r.options.EntityType != entityType {
		return contractsv1.Envelope{}, fmt.Errorf("entity_type %q does not match configured type %q", entityType, r.options.EntityType)
	}
	channel, err := getString("type")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	eventTime, arrival, err := simulatorEventTimes(event)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	data, err := simulatorEventData(event, strings.TrimPrefix(channel, entityType+"."))
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	return contractsv1.Envelope{
		ID: id, Type: r.eventTypePrefix(entityType, channel) + channel + ".observed", SchemaVersion: "1.0",
		TenantID: r.options.TenantID, Source: r.options.Source, PartitionKey: entityID,
		Entity: contractsv1.EntityRef{Type: entityType, ID: entityID}, EventTime: eventTime,
		ObservedAt: &arrival, IngestedAt: arrival, Classification: contractsv1.ClassificationInternal,
		Quality: []contractsv1.QualityFlag{}, Data: data,
	}, nil
}

// simulatorEventFields returns the event object, rejecting unknown fields at
// both the record and event level.
func simulatorEventFields(record map[string]any) (map[string]any, error) {
	allowed := map[string]bool{"record_type": true, "event": true}
	for key := range record {
		if !allowed[key] {
			return nil, fmt.Errorf("unknown event field %q", key)
		}
	}
	event, ok := record["event"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event is required")
	}
	eventAllowed := map[string]bool{"id": true, "entity_type": true, "entity_id": true, "type": true, "event_time": true, "arrival_time": true, "value": true, "unit": true}
	for key := range event {
		if !eventAllowed[key] {
			return nil, fmt.Errorf("unknown event field %q", key)
		}
	}
	return event, nil
}

// eventTypePrefix is the configured prefix, defaulting to the entity type,
// unless the channel already carries it.
func (r *SimulatorJSONLReplay) eventTypePrefix(entityType, channel string) string {
	prefix := r.options.EventTypePrefix
	if prefix == "" {
		prefix = entityType + "."
	}
	if strings.HasPrefix(channel, prefix) {
		return ""
	}
	return prefix
}

func simulatorEventTimes(event map[string]any) (time.Time, time.Time, error) {
	eventTime, err := parseSimulatorTime(event, "event_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	arrival, err := parseSimulatorTime(event, "arrival_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if arrival.Before(eventTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("arrival precedes event time")
	}
	return eventTime, arrival, nil
}

// simulatorEventData maps the event value to its data field. The
// channel-to-field mapping is DATA (simulator_data.json): the heartbeat
// sentinel (present, empty target) emits no data field, and an ABSENT channel
// falls back to data["value"].
func simulatorEventData(event map[string]any, channelName string) (map[string]any, error) {
	data := make(map[string]any)
	if value, ok := event["value"]; ok {
		fields, err := channelFields()
		if err != nil {
			return nil, fmt.Errorf("load simulator channel fields: %w", err)
		}
		target, known := fields[channelName]
		switch {
		case known && target == "":
			// Heartbeats (and other no-data channels) carry no field.
		case !known || target == "value":
			data["value"] = value
		default:
			data[target] = value
		}
	}
	if unit, ok := event["unit"].(string); ok && unit != "" {
		data["unit"] = unit
	}
	if channelName == "mode" {
		if value, ok := event["value"]; ok {
			data["mode"] = value
		}
	}
	return data, nil
}

func parseSimulatorTime(record map[string]any, key string) (time.Time, error) {
	value, ok := record[key].(string)
	if !ok || value == "" {
		return time.Time{}, fmt.Errorf("%s is required", key)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed.UTC(), nil
}

func validateSimulatorControl(recordType string, record map[string]any) error {
	if recordType == "runtime_config" {
		if len(record) != 4 {
			return fmt.Errorf("runtime_config contains unknown fields")
		}
		version, versionOK := record["runtime_version"].(string)
		storageVersion, storageOK := integerValue(record["storage_schema_version"])
		maxEpisodes, maxOK := integerValue(record["max_episodes_per_hour"])
		if !versionOK || version == "" || !storageOK || storageVersion < 1 || !maxOK || maxEpisodes < 1 || maxEpisodes > 10000 {
			return fmt.Errorf("incomplete runtime_config")
		}
		return nil
	}
	if len(record) != 2 {
		return fmt.Errorf("trace_end contains unknown fields")
	}
	if _, err := parseSimulatorTime(record, "until"); err != nil {
		return err
	}
	return nil
}

func validateModelActivation(record map[string]any) (time.Time, error) {
	allowed := map[string]bool{"record_type": true, "recorded_time": true, "mode": true, "model": true}
	for key := range record {
		if !allowed[key] {
			return time.Time{}, fmt.Errorf("model_activation contains unknown field %q", key)
		}
	}
	if len(record) != 4 {
		return time.Time{}, fmt.Errorf("model_activation is incomplete")
	}
	recorded, err := parseSimulatorTime(record, "recorded_time")
	if err != nil {
		return time.Time{}, err
	}
	mode, ok := record["mode"].(string)
	if !ok || (mode != "continue" && mode != "reset") {
		return time.Time{}, fmt.Errorf("model_activation mode must be continue or reset")
	}
	model, ok := record["model"].(map[string]any)
	if !ok {
		return time.Time{}, fmt.Errorf("model_activation model is required")
	}
	for _, key := range []string{"schema_version", "id", "version", "entity_type", "lateness_allowance", "inputs", "windows", "facts", "states", "episode_types", "cognition_triggers", "budgets"} {
		if _, ok := model[key]; !ok {
			return time.Time{}, fmt.Errorf("model_activation model.%s is required", key)
		}
	}
	if model["schema_version"] != "0.1.0" {
		return time.Time{}, fmt.Errorf("model_activation model.schema_version must be 0.1.0")
	}
	return recorded, nil
}

func integerValue(value any) (int64, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int64(number)) {
		return 0, false
	}
	return int64(number), true
}

func loadLineCheckpoint(ctx context.Context, db *storage.DB, connectorID string) (int, error) {
	var blob []byte
	if err := db.QueryRowContext(ctx, "SELECT checkpoint_blob FROM connector_checkpoints WHERE connector_id = ?", connectorID).Scan(&blob); err != nil {
		return 0, nil
	}
	var checkpoint struct {
		LastLine int `json:"last_line"`
	}
	if err := json.Unmarshal(blob, &checkpoint); err != nil {
		return 0, fmt.Errorf("decode connector checkpoint: %w", err)
	}
	return checkpoint.LastLine, nil
}

func saveLineCheckpoint(ctx context.Context, db *storage.DB, connectorID string, line int, now time.Time) error {
	blob, err := json.Marshal(map[string]any{"version": 1, "last_line": line})
	if err != nil {
		return fmt.Errorf("encode connector checkpoint: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at)
		VALUES (?, 'simulator-jsonl', 1, ?, ?)
		ON CONFLICT(connector_id) DO UPDATE SET checkpoint_blob = excluded.checkpoint_blob, updated_at = excluded.updated_at`,
		connectorID, blob, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save connector checkpoint: %w", err)
	}
	return nil
}
