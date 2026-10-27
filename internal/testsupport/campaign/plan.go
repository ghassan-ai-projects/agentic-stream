package campaign

import (
	"encoding/json"
	"fmt"
	"os"
)

// PreparePlan copies a run's session plan into dir with the host socket and
// the accepted policy digests set, and returns the copy's path. Every other
// field, the injected disturbances among them, passes through undecoded: the
// consumer side never reads what a run injects.
func (c Campaign) PreparePlan(run Run, dir, hostSocket string, policies []string) (string, error) {
	data, err := readIn(c.Dir, run.SessionPlan)
	if err != nil {
		return "", fmt.Errorf("run %d: %w", run.Index, err)
	}
	var plan map[string]json.RawMessage
	if err := json.Unmarshal(data, &plan); err != nil {
		return "", fmt.Errorf("run %d: decode the session plan: %w", run.Index, err)
	}
	if plan["host_socket"], err = json.Marshal(hostSocket); err != nil {
		return "", fmt.Errorf("run %d: encode the host socket: %w", run.Index, err)
	}
	if plan["policies"], err = json.Marshal(policies); err != nil {
		return "", fmt.Errorf("run %d: encode the policies: %w", run.Index, err)
	}
	return writeIn(dir, run.SessionPlan, plan)
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
