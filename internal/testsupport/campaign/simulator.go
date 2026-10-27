package campaign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Simulator runs streams-simulator sessions and grades them.
type Simulator struct {
	Binary     string
	Repository string
}

// Session is one running plantwire connect session.
type Session struct {
	Out    string
	cmd    *exec.Cmd
	output *bytes.Buffer
}

// Connect starts the session of a prepared plan, writing its run into out.
func (s Simulator) Connect(plan, out string) (*Session, error) {
	output := &bytes.Buffer{}
	session := &Session{Out: out, output: output, cmd: &exec.Cmd{Path: s.Binary, Stdout: output, Stderr: output,
		Args: []string{s.Binary, "plantwire", "connect", "--plan", plan, "--domains-dir", s.station(),
			"--kinds-dir", filepath.Join(s.station(), "kinds"), "--adapters-dir", s.adapters(), "--slice-s", "60", "--out", out}}}
	if err := session.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start the session of %s: %w", plan, err)
	}
	return session, nil
}

// Stop interrupts the session, the operator's Stop that ends a run cleanly,
// and waits for it.
func (s *Session) Stop() error {
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("interrupt the session: %w", err)
	}
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("the session ended with %w: %s", err, s.output.Bytes())
	}
	return nil
}

// Score grades a stopped run against its sealed label with the simulator's
// scorer.
func (s Simulator) Score(runDir string) (json.RawMessage, error) {
	score := exec.Cmd{Path: s.Binary, Args: []string{s.Binary, "score", "--run", filepath.Join(runDir, "run.json"), "--domains-dir", s.station(),
		"--adapters-dir", s.adapters()}}
	output, err := score.Output()
	if err != nil {
		return nil, fmt.Errorf("score %s: %w", runDir, err)
	}
	if !json.Valid(output) {
		return nil, fmt.Errorf("score %s: %w: %s", runDir, errNotJSON, output)
	}
	return output, nil
}

func (s Simulator) station() string { return filepath.Join(s.Repository, "examples", "pump-station") }

func (s Simulator) adapters() string { return filepath.Join(s.Repository, "adapters") }
