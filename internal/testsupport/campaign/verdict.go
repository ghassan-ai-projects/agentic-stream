package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const verdictFile = "verdict.json"

const detectionsQuery = `SELECT s.entity_id, s.first_event_time, COALESCE((SELECT json_extract(d.raw_json, '$.diagnosis_code')
		FROM decisions d WHERE d.situation_id = s.situation_id AND d.validation_status = 'accepted'
		ORDER BY d.situation_version DESC, d.created_at DESC LIMIT 1), '')
	FROM situations s WHERE s.created_at >= ?1 ORDER BY s.first_event_time, s.entity_id`

const actionsQuery = `SELECT 'sha256:' || lower(hex(c.idempotency_key)), c.effector_route, c.normalized_target, s.latest_event_time, c.status
	FROM commands c JOIN intents i ON i.intent_id = c.intent_id JOIN situations s ON s.situation_id = i.situation_id
	WHERE c.created_at >= ?1 ORDER BY c.created_at, c.command_id`

type verdict struct {
	SchemaVersion string      `json:"schema_version"`
	RunID         string      `json:"run_id"`
	Consumer      consumer    `json:"consumer"`
	Detections    []detection `json:"detections"`
	Actions       []action    `json:"actions"`
}

type consumer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arm     string `json:"arm"`
}

type detection struct {
	EntityID   string `json:"entity_id"`
	DetectedAt string `json:"detected_at"`
	Label      string `json:"label,omitempty"`
}

type action struct {
	CommandID       string `json:"command_id"`
	Effector        string `json:"effector"`
	EntityID        string `json:"entity_id"`
	IssuedAt        string `json:"issued_at"`
	OutcomeBelieved string `json:"outcome_believed"`
}

// WriteVerdict writes the run's consumer verdict beside its run file: the
// situations opened and the commands sent since the run started, in the
// simulator's neutral vocabulary.
func WriteVerdict(ctx context.Context, db *sql.DB, runDir, arm string, since time.Time) error {
	runID, err := readRunID(runDir)
	if err != nil {
		return err
	}
	v := verdict{SchemaVersion: schemaVersion, RunID: runID, Consumer: consumer{Name: "agentic-stream", Version: "rule-promotion", Arm: arm}}
	from := kernel.FormatTime(since)
	if v.Detections, err = readDetections(ctx, db, from); err != nil {
		return err
	}
	if v.Actions, err = readActions(ctx, db, from); err != nil {
		return err
	}
	_, err = writeIn(runDir, verdictFile, v)
	return err
}

func readRunID(runDir string) (string, error) {
	data, err := readIn(runDir, "run.json")
	if err != nil {
		return "", err
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(data, &run); err != nil {
		return "", fmt.Errorf("decode the run file in %s: %w", runDir, err)
	}
	return run.RunID, nil
}

func readDetections(ctx context.Context, db *sql.DB, from string) ([]detection, error) {
	detections, err := storage.QueryAll(ctx, db, "detection", scanDetection, detectionsQuery, from)
	if err != nil {
		return nil, fmt.Errorf("read the detections: %w", err)
	}
	return detections, nil
}

func scanDetection(rows *sql.Rows) (detection, error) {
	var d detection
	if err := rows.Scan(&d.EntityID, &d.DetectedAt, &d.Label); err != nil {
		return d, fmt.Errorf("scan a detection: %w", err)
	}
	d.EntityID = instanceOf(d.EntityID)
	return d, nil
}

func readActions(ctx context.Context, db *sql.DB, from string) ([]action, error) {
	actions, err := storage.QueryAll(ctx, db, "action", scanAction, actionsQuery, from)
	if err != nil {
		return nil, fmt.Errorf("read the actions: %w", err)
	}
	return actions, nil
}

func scanAction(rows *sql.Rows) (action, error) {
	var a action
	var status string
	if err := rows.Scan(&a.CommandID, &a.Effector, &a.EntityID, &a.IssuedAt, &status); err != nil {
		return a, fmt.Errorf("scan an action: %w", err)
	}
	a.EntityID, a.OutcomeBelieved = instanceOf(a.EntityID), believedOutcome(status)
	return a, nil
}

func instanceOf(entityID string) string { return strings.Replace(entityID, ":", "/", 1) }

func believedOutcome(status string) string {
	switch status {
	case "succeeded", "failed":
		return status
	default:
		return "unknown"
	}
}
