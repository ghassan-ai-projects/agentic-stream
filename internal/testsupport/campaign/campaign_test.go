package campaign

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const disturbances = `[{"at_s":3600,"instance":"site-a/pump-3","fault":"bearing_wear"}]`

func writeCampaign(t *testing.T) Campaign {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "campaign.json", `{"schema_version":"0.1","domain":"pump-station","seed":1,"runs":[
		{"index":1,"phase":"learn","domain":"pump-station","seed":11,"duration_s":7200,"session_plan":"run-0001.session-plan.json"},
		{"index":2,"phase":"holdout","domain":"pump-station","seed":12,"duration_s":7200,"session_plan":"run-0002.session-plan.json"}]}`)
	write(t, dir, "run-0001.session-plan.json", `{"schema_version":"0.1","domain":"pump-station","seed":11,"host_socket":"/tmp/x.sock","disturbances":`+disturbances+`}`)
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadReadsTheRunsOfACampaign(t *testing.T) {
	t.Parallel()
	loaded := writeCampaign(t)
	if len(loaded.Runs) != 2 || loaded.Runs[1].Phase != "holdout" || loaded.Runs[0].Duration() != 2*time.Hour {
		t.Fatalf("campaign = %+v, want two runs of two hours", loaded)
	}
}

func TestLoadRefusesAnotherSchemaOrNoRuns(t *testing.T) {
	t.Parallel()
	for want, content := range map[string]string{`schema "0.2"`: `{"schema_version":"0.2","runs":[{"index":1}]}`, "0 runs": `{"schema_version":"0.1","runs":[]}`, "decode the campaign": `{`} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			write(t, dir, "campaign.json", content)
			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("Load = %v, want %q", err, want)
			}
		})
	}
	if _, err := Load(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load of a folder without a campaign = %v, want not exist", err)
	}
}

func TestArmsMatchThePlan(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]Arm{"0": {Static: true}, "A": {}, "B": {Learning: true}, "C": {Learning: true}} {
		arm, err := ArmNamed(name)
		if err != nil || arm.Static != want.Static || arm.Learning != want.Learning || arm.Approver == nil {
			t.Fatalf("arm %s = %+v (%v), want static %v learning %v", name, arm, err, want.Static, want.Learning)
		}
	}
	if _, err := ArmNamed("D"); !errors.Is(err, errUnknownArm) {
		t.Fatalf("arm D: %v, want unknown", err)
	}
}

func TestAPreparedPlanKeepsTheDisturbancesUnreadAndSetsTheHost(t *testing.T) {
	t.Parallel()
	loaded := writeCampaign(t)
	path, err := loaded.PreparePlan(loaded.Runs[0], t.TempDir(), "/tmp/as/host.sock", []string{"sha256:ab"}, worldEpoch.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	data, err := readIn(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]json.RawMessage
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	if string(plan["host_socket"]) != `"/tmp/as/host.sock"` || string(plan["disturbances"]) != disturbances || plan["policies"] == nil || string(plan["start_s"]) != "3600" {
		t.Fatalf("plan = %s", data)
	}
	if _, err := loaded.PreparePlan(loaded.Runs[1], t.TempDir(), "/tmp/as/host.sock", nil, worldEpoch); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("PreparePlan of a missing plan = %v, want not exist", err)
	}
}
