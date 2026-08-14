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
	scanner := bufio.NewScanner(f)
	line := 0
	appended := 0
	records := 0
	configured := false
	ended := false
	var lastRecorded time.Time
	var batch []contractsv1.Envelope
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		positions, appendErr := r.log.Append(ctx, r.options.TenantID, batch)
		if appendErr != nil {
			return fmt.Errorf("append simulator batch: %w", appendErr)
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
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return appended, fmt.Errorf("parse simulator line %d: %w", line, err)
		}
		recordType, _ := record["record_type"].(string)
		records++
		if records == 1 && recordType != "runtime_config" {
			return appended, fmt.Errorf("validate simulator line %d: runtime_config must be first", line)
		}
		if ended {
			return appended, fmt.Errorf("validate simulator line %d: record follows trace_end", line)
		}
		switch recordType {
		case "runtime_config":
			if configured {
				return appended, fmt.Errorf("validate simulator line %d: duplicate runtime_config", line)
			}
			if err := validateSimulatorControl(recordType, record); err != nil {
				return appended, fmt.Errorf("validate simulator line %d: %w", line, err)
			}
			configured = true
		case "event":
			env, convertErr := r.convertEvent(record)
			if convertErr != nil {
				return appended, fmt.Errorf("convert simulator line %d: %w", line, convertErr)
			}
			recorded := env.ObservedAt
			if recorded == nil {
				return appended, fmt.Errorf("event arrival_time is required")
			}
			if !lastRecorded.IsZero() && !recorded.After(lastRecorded) {
				return appended, fmt.Errorf("event recorded time must be strictly increasing")
			}
			lastRecorded = recorded.UTC()
			if line > start {
				batch = append(batch, env)
				if len(batch) == 100 {
					if err := flush(); err != nil {
						return appended, fmt.Errorf("append simulator batch: %w", err)
					}
				}
			}
		case "model_activation":
			recorded, err := validateModelActivation(record)
			if err != nil {
				return appended, fmt.Errorf("validate simulator line %d: %w", line, err)
			}
			if !lastRecorded.IsZero() && !recorded.After(lastRecorded) {
				return appended, fmt.Errorf("recorded time must be strictly increasing")
			}
			lastRecorded = recorded
		case "trace_end":
			if err := validateSimulatorControl(recordType, record); err != nil {
				return appended, fmt.Errorf("validate simulator line %d: %w", line, err)
			}
			until, _ := parseSimulatorTime(record, "until")
			if !lastRecorded.IsZero() && !until.After(lastRecorded) {
				return appended, fmt.Errorf("trace_end until must be later than every recorded input")
			}
			ended = true
		default:
			return appended, fmt.Errorf("unknown simulator record_type %q", recordType)
		}
	}
	if err := scanner.Err(); err != nil {
		return appended, fmt.Errorf("read simulator trace: %w", err)
	}
	if !configured || !ended {
		return appended, fmt.Errorf("simulator trace requires runtime_config first and trace_end last")
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
	allowed := map[string]bool{"record_type": true, "event": true}
	for key := range record {
		if !allowed[key] {
			return contractsv1.Envelope{}, fmt.Errorf("unknown event field %q", key)
		}
	}
	event, ok := record["event"].(map[string]any)
	if !ok {
		return contractsv1.Envelope{}, fmt.Errorf("event is required")
	}
	eventAllowed := map[string]bool{"id": true, "entity_type": true, "entity_id": true, "type": true, "event_time": true, "arrival_time": true, "value": true, "unit": true}
	for key := range event {
		if !eventAllowed[key] {
			return contractsv1.Envelope{}, fmt.Errorf("unknown event field %q", key)
		}
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
	eventTypePrefix := r.options.EventTypePrefix
	if eventTypePrefix == "" {
		eventTypePrefix = entityType + "."
	}
	if strings.HasPrefix(channel, eventTypePrefix) {
		eventTypePrefix = ""
	}
	eventTime, err := parseSimulatorTime(event, "event_time")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	arrival, err := parseSimulatorTime(event, "arrival_time")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	if arrival.Before(eventTime) {
		return contractsv1.Envelope{}, fmt.Errorf("arrival precedes event time")
	}
	data := make(map[string]any)
	channelName := strings.TrimPrefix(channel, entityType+".")
	if value, ok := event["value"]; ok {
		switch channelName {
		case "vibration":
			data["rms_mm_s"] = value
		case "temperature":
			data["celsius"] = value
		case "current":
			data["amps"] = value
		case "dissolved_oxygen", "ammonia":
			data["mg_l"] = value
		case "water_temperature":
			data["celsius"] = value
		case "ph":
			data["ph"] = value
		case "aerator_current":
			data["ampere"] = value
		case "feeding_event":
			data["load"] = value
		case "discharge_pressure":
			data["kpa"] = value
		case "flow_rate":
			data["l_s"] = value
		case "tank_level":
			data["percent"] = value
		case "turbidity":
			data["ntu"] = value
		case "demand_event":
			data["magnitude"] = value
		case "humidity", "leaf_wetness", "vent_position":
			data["percent"] = value
		case "air_temp":
			data["celsius"] = value
		case "co2":
			data["umol_mol"] = value
		case "par_light":
			data["value"] = value
		case "vent_event":
			data["magnitude"] = value
		case "heartbeat":
			// Heartbeats carry no data field.
		default:
			data["value"] = value
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
	return contractsv1.Envelope{
		ID: id, Type: eventTypePrefix + channel + ".observed", SchemaVersion: "1.0",
		TenantID: r.options.TenantID, Source: r.options.Source, PartitionKey: entityID,
		Entity: contractsv1.EntityRef{Type: entityType, ID: entityID}, EventTime: eventTime,
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
