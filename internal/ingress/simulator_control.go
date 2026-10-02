package ingress

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

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
