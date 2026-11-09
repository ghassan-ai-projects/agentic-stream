package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const verdictFile = "verdict.json"

const decidedSQL = `FROM decisions d JOIN episodes e ON e.episode_id = d.episode_id
	JOIN situations s ON s.situation_id = e.situation_id
	JOIN situation_versions v ON v.situation_id = e.situation_id AND v.version = e.situation_version
	WHERE d.validation_status = 'accepted' AND e.dispatch_policy != 'shadow' AND v.event_horizon >= ?1 AND v.event_horizon <= ?2`

const detectionsQuery = `SELECT entity_id, decided_at, label FROM (
	SELECT s.entity_id AS entity_id, v.event_horizon AS decided_at, COALESCE(json_extract(d.raw_json, '$.diagnosis_code'), '') AS label,
		ROW_NUMBER() OVER (PARTITION BY s.entity_id, json_extract(d.raw_json, '$.diagnosis_code') ORDER BY v.event_horizon, d.created_at, d.decision_id) AS n
	` + decidedSQL + `) WHERE n = 1 ORDER BY decided_at, entity_id`

const actionsQuery = `SELECT 'sha256:' || lower(hex(c.idempotency_key)), c.effector_route, c.normalized_target, v.event_horizon, c.status
	FROM commands c JOIN intents i ON i.intent_id = c.intent_id JOIN decisions d ON d.decision_id = i.decision_id
	JOIN episodes e ON e.episode_id = d.episode_id
	JOIN situation_versions v ON v.situation_id = e.situation_id AND v.version = e.situation_version
	WHERE v.event_horizon >= ?1 AND v.event_horizon <= ?2 ORDER BY v.event_horizon, c.command_id`

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
	EntityID           string `json:"entity_id"`
	DetectedAt         string `json:"detected_at"`
	Label              string `json:"label,omitempty"`
	PredictedFailureAt string `json:"predicted_failure_at,omitempty"`
	situationEntity    string
}

type action struct {
	CommandID       string `json:"command_id"`
	Effector        string `json:"effector"`
	EntityID        string `json:"entity_id"`
	IssuedAt        string `json:"issued_at"`
	OutcomeBelieved string `json:"outcome_believed"`
}

// Subjects names what a verdict reports in the simulator's vocabulary: the
// instance and label of a detection from its Situation's entity and the
// decision's diagnosis, and the instance and effector an action reached from
// its route and normalized target. A detection mapped to no instance is left
// out (a diagnosis that names nothing). One mapped to an instance without a
// label is a symptom: it is not a row of its own while a labeled detection of
// that instance exists, whose detection time it moves earlier; otherwise its
// earliest occurrence is reported without a label. Prognosis names, per
// label, the Situation fact that holds the seconds until the detected
// instance fails if nobody acts (an extrapolate operator's output): a
// detection of that label carries the failure time its Situation predicted
// then, and each later prediction, at most one per PredictionSpacing of event
// time, is reported as a further detection of the same instance and label.
type Subjects struct {
	Detected  func(entityID, diagnosis string) (instance, label string)
	Acted     func(route, target string) (instance, effector string)
	Prognosis map[string]string
}

// SameNames is the campaign's subjects: an entity "site-a:pump-1" is the
// instance "site-a/pump-1", a diagnosis is the label and a route is the
// effector.
var SameNames = Subjects{
	Detected: func(entityID, diagnosis string) (string, string) { return instanceOf(entityID), diagnosis },
	Acted:    func(route, target string) (string, string) { return instanceOf(target), route },
}

// WriteVerdict writes the campaign run's consumer verdict with SameNames as
// its subjects.
func WriteVerdict(ctx context.Context, db *sql.DB, runDir, arm string, window Window) error {
	return WriteVerdictOf(ctx, db, runDir, window, Verdict{Version: "rule-promotion", Arm: arm, Subjects: SameNames})
}

// Verdict is the consumer version and comparison arm a verdict names and the
// subjects its rows report.
type Verdict struct {
	Version, Arm string
	Subjects     Subjects
}

// WriteVerdictOf writes the run's consumer verdict beside its run file, in the
// simulator's neutral vocabulary. Everything is placed in simulated time: a
// detection is the first decision of each diagnosis on an entity whose
// Situation version's event horizon falls inside the run's window (a first
// "unknown" does not hide the cause a later decision names), and an action is a command
// whose deciding episode does, issued at that horizon.
func WriteVerdictOf(ctx context.Context, db *sql.DB, runDir string, window Window, of Verdict) error {
	runID, err := readRunID(runDir)
	if err != nil {
		return err
	}
	v := verdict{SchemaVersion: schemaVersion, RunID: runID, Consumer: consumer{Name: "agentic-stream", Version: of.Version, Arm: of.Arm}}
	if err := v.gather(ctx, db, window, of.Subjects); err != nil {
		return err
	}
	_, err = writeIn(runDir, verdictFile, v)
	return err
}

func (v *verdict) gather(ctx context.Context, db *sql.DB, window Window, subjects Subjects) error {
	if err := v.read(ctx, db, window); err != nil {
		return err
	}
	subjects.place(v)
	var err error
	v.Detections, err = subjects.predict(ctx, db, window.Until, v.Detections)
	return err
}

func (v *verdict) read(ctx context.Context, db *sql.DB, window Window) error {
	from, until := kernel.FormatTime(window.From), kernel.FormatTime(window.Until)
	var err error
	if v.Detections, err = readVerdictRows(ctx, db, "detection", scanDetection, detectionsQuery, from, until); err != nil {
		return err
	}
	v.Actions, err = readVerdictRows(ctx, db, "action", scanAction, actionsQuery, from, until)
	return err
}

func (s Subjects) place(v *verdict) {
	v.Detections = withSymptomsFolded(s.detections(v.Detections))
	for i, a := range v.Actions {
		v.Actions[i].EntityID, v.Actions[i].Effector = s.Acted(a.Effector, a.EntityID)
	}
}

func (s Subjects) detections(decided []detection) []detection {
	placed := []detection{}
	for _, d := range decided {
		instance, label := s.Detected(d.EntityID, d.Label)
		if instance != "" {
			placed = append(placed, detection{EntityID: instance, DetectedAt: d.DetectedAt, Label: label, situationEntity: d.EntityID})
		}
	}
	return placed
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

func readVerdictRows[T any](ctx context.Context, db *sql.DB, what string, scan func(*sql.Rows) (T, error), query string, window ...any) ([]T, error) {
	rows, err := storage.QueryAll(ctx, db, what, scan, query, window...)
	if err != nil {
		return nil, fmt.Errorf("read the verdict's %ss: %w", what, err)
	}
	return rows, nil
}

func scanDetection(rows *sql.Rows) (detection, error) {
	var d detection
	if err := rows.Scan(&d.EntityID, &d.DetectedAt, &d.Label); err != nil {
		return d, fmt.Errorf("scan a detection: %w", err)
	}
	return d, nil
}

func scanAction(rows *sql.Rows) (action, error) {
	var a action
	var status string
	if err := rows.Scan(&a.CommandID, &a.Effector, &a.EntityID, &a.IssuedAt, &status); err != nil {
		return a, fmt.Errorf("scan an action: %w", err)
	}
	a.OutcomeBelieved = believedOutcome(status)
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
