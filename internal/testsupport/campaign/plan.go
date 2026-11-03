package campaign

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

var worldEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Window is the span of simulated time one run's world covers.
type Window struct{ From, Until time.Time }

// Window is the run's simulated span: its world starts the plan's start_s
// seconds after 2026-01-01 and lasts the run's duration.
func (c Campaign) Window(run Run) (Window, error) {
	data, err := readIn(c.Dir, run.SessionPlan)
	if err != nil {
		return Window{}, fmt.Errorf("run %d: %w", run.Index, err)
	}
	var plan struct {
		StartS float64 `json:"start_s"`
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		return Window{}, fmt.Errorf("run %d: decode the session plan's start: %w", run.Index, err)
	}
	from := worldEpoch.Add(time.Duration(plan.StartS * float64(time.Second)))
	return Window{From: from, Until: from.Add(run.Duration())}, nil
}

// PreparePlan copies a run's session plan into dir with the host socket, the
// accepted policy digests and the world's start set, and returns the copy's
// path. Every other field, the injected disturbances among them, passes
// through undecoded: the consumer side never reads what a run injects.
func (c Campaign) PreparePlan(run Run, dir, hostSocket string, policies []string, start time.Time) (string, error) {
	data, err := readIn(c.Dir, run.SessionPlan)
	if err != nil {
		return "", fmt.Errorf("run %d: %w", run.Index, err)
	}
	var plan map[string]json.RawMessage
	if err := json.Unmarshal(data, &plan); err != nil {
		return "", fmt.Errorf("run %d: decode the session plan: %w", run.Index, err)
	}
	fields := map[string]any{"host_socket": hostSocket, "policies": policies, "start_s": int64(start.Sub(worldEpoch) / time.Second)}
	if err := setFields(plan, fields); err != nil {
		return "", fmt.Errorf("run %d: %w", run.Index, err)
	}
	return writeIn(dir, run.SessionPlan, plan)
}

func setFields(plan map[string]json.RawMessage, fields map[string]any) error {
	for name, value := range fields {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode the plan's %s: %w", name, err)
		}
		plan[name] = encoded
	}
	return nil
}

func writeIn(dir, name string, document any) (string, error) {
	data, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", name, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(name, data, 0o600); err != nil {
		return "", fmt.Errorf("write %s in %s: %w", name, dir, err)
	}
	return dir + string(os.PathSeparator) + name, nil
}
