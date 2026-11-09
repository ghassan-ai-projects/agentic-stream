package campaign

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// PredictionSpacing is the least event time between two predictions a verdict
// reports for one detection.
const PredictionSpacing = 10 * time.Minute

const predictionsQuery = `SELECT v.event_horizon, COALESCE(json_extract(CAST(v.snapshot_json AS TEXT), ?1), 0)
	FROM situation_versions v JOIN situations s ON s.situation_id = v.situation_id
	WHERE s.entity_id = ?2 AND v.event_horizon >= ?3 AND v.event_horizon <= ?4 ORDER BY v.event_horizon, v.version`

type prediction struct {
	at        time.Time
	failureAt time.Time
}

func (s Subjects) predict(ctx context.Context, db *sql.DB, until time.Time, detections []detection) ([]detection, error) {
	predicted := []detection{}
	for _, d := range detections {
		rows, err := s.predictionsOf(ctx, db, until, d)
		if err != nil {
			return nil, err
		}
		predicted = append(predicted, rows...)
	}
	slices.SortStableFunc(predicted, func(a, b detection) int {
		return cmp.Or(cmp.Compare(a.DetectedAt, b.DetectedAt), cmp.Compare(a.EntityID, b.EntityID))
	})
	return predicted, nil
}

func (s Subjects) predictionsOf(ctx context.Context, db *sql.DB, until time.Time, d detection) ([]detection, error) {
	fact, prognosed := s.Prognosis[d.Label]
	if !prognosed {
		return []detection{d}, nil
	}
	track, err := readPredictions(ctx, db, fact, d, until)
	if err != nil {
		return nil, err
	}
	return d.withPredictions(track), nil
}

func readPredictions(ctx context.Context, db *sql.DB, fact string, d detection, until time.Time) ([]prediction, error) {
	path := fmt.Sprintf(`$.facts."facts.%s"`, fact)
	rows, err := readVerdictRows(ctx, db, "prediction", scanPrediction, predictionsQuery, path, d.situationEntity, d.DetectedAt, kernel.FormatTime(until))
	if err != nil {
		return nil, err
	}
	return spaced(rows), nil
}

func scanPrediction(rows *sql.Rows) (prediction, error) {
	var horizon string
	var seconds float64
	if err := rows.Scan(&horizon, &seconds); err != nil {
		return prediction{}, fmt.Errorf("scan a prediction: %w", err)
	}
	at, err := kernel.ParseTime(horizon)
	if err != nil {
		return prediction{}, fmt.Errorf("read a prediction's horizon %q: %w", horizon, err)
	}
	if seconds <= 0 {
		return prediction{at: at}, nil
	}
	return prediction{at: at, failureAt: at.Add(time.Duration(seconds * float64(time.Second)))}, nil
}

func spaced(track []prediction) []prediction {
	kept := []prediction{}
	for _, p := range track {
		if p.failureAt.IsZero() || len(kept) > 0 && p.at.Sub(kept[len(kept)-1].at) < PredictionSpacing {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

func (d detection) withPredictions(track []prediction) []detection {
	rows := []detection{d}
	for _, p := range track {
		at, failureAt := kernel.FormatTime(p.at), kernel.FormatTime(p.failureAt)
		if at == d.DetectedAt {
			rows[0].PredictedFailureAt = failureAt
			continue
		}
		rows = append(rows, detection{EntityID: d.EntityID, DetectedAt: at, Label: d.Label, PredictedFailureAt: failureAt})
	}
	return rows
}
