package ingress

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

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
	if options.EntityType == "" {
		options.EntityType = "motor"
	}
	if options.EventTypePrefix == "" {
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
	scanner := bufio.NewScanner(f)
	line := 0
	appended := 0
	var batch []contractsv1.Envelope
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		positions, appendErr := r.log.Append(ctx, r.options.TenantID, batch)
		if appendErr != nil {
			return appendErr
		}
		for _, position := range positions {
			if position >= 0 {
				appended++
			}
		}
		batch = batch[:0]
		return nil
	}
	for scanner.Scan() {
		line++
		if line <= start || strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return appended, fmt.Errorf("parse simulator line %d: %w", line, err)
		}
		recordType, _ := record["record_type"].(string)
		switch recordType {
		case "runtime_config", "trace_end":
			if err := validateSimulatorControl(recordType, record); err != nil {
				return appended, fmt.Errorf("validate simulator line %d: %w", line, err)
			}
		case "event":
			env, convertErr := r.convertEvent(record)
			if convertErr != nil {
				return appended, fmt.Errorf("convert simulator line %d: %w", line, convertErr)
			}
			batch = append(batch, env)
			if len(batch) == 100 {
				if err := flush(); err != nil {
					return appended, fmt.Errorf("append simulator batch: %w", err)
				}
			}
		default:
			return appended, fmt.Errorf("unknown simulator record_type %q", recordType)
		}
	}
	if err := scanner.Err(); err != nil {
		return appended, fmt.Errorf("read simulator trace: %w", err)
	}
	if err := flush(); err != nil {
		return appended, fmt.Errorf("append simulator batch: %w", err)
	}
	if err := saveLineCheckpoint(ctx, r.db, r.connectorID, line, r.clk.Now()); err != nil {
		return appended, err
	}
	return appended, nil
}

func (r *SimulatorJSONLReplay) convertEvent(record map[string]any) (contractsv1.Envelope, error) {
	allowed := map[string]bool{"record_type": true, "event.id": true, "event.entity_type": true, "event.entity_id": true, "event.type": true, "event.event_time": true, "event.arrival_time": true, "event.value": true, "event.unit": true}
	for key := range record {
		if !allowed[key] {
			return contractsv1.Envelope{}, fmt.Errorf("unknown event field %q", key)
		}
	}
	getString := func(key string) (string, error) {
		value, ok := record[key].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return value, nil
	}
	id, err := getString("event.id")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	entityID, err := getString("event.entity_id")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	if _, err := getString("event.entity_type"); err != nil {
		return contractsv1.Envelope{}, err
	}
	channel, err := getString("event.type")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	eventTime, err := parseSimulatorTime(record, "event.event_time")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	arrival, err := parseSimulatorTime(record, "event.arrival_time")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	if arrival.Before(eventTime) {
		return contractsv1.Envelope{}, fmt.Errorf("arrival precedes event time")
	}
	data := make(map[string]any)
	if value, ok := record["event.value"]; ok {
		switch channel {
		case "vibration":
			data["rms_mm_s"] = value
		case "temperature":
			data["celsius"] = value
		case "current":
			data["amps"] = value
		default:
			data["value"] = value
		}
	}
	if unit, ok := record["event.unit"].(string); ok && unit != "" {
		data["unit"] = unit
	}
	if channel == "mode" {
		if value, ok := record["event.value"]; ok {
			data["mode"] = value
		}
	}
	return contractsv1.Envelope{
		ID: id, Type: r.options.EventTypePrefix + channel + ".observed", SchemaVersion: "1.0",
		TenantID: r.options.TenantID, Source: r.options.Source, PartitionKey: entityID,
		Entity: contractsv1.EntityRef{Type: r.options.EntityType, ID: entityID}, EventTime: eventTime,
		ObservedAt: &arrival, IngestedAt: arrival, Classification: contractsv1.ClassificationInternal,
		Quality: []contractsv1.QualityFlag{}, Data: data,
	}, nil
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
		if record["runtime_version"] == nil || record["storage_schema_version"] == nil || record["max_episodes_per_hour"] == nil {
			return fmt.Errorf("incomplete runtime_config")
		}
		return nil
	}
	if _, err := parseSimulatorTime(record, "until"); err != nil {
		return err
	}
	return nil
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
