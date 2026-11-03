package main

import (
	"strings"
	"testing"
)

func TestServeRefusesAPlaybookWithoutASpec(t *testing.T) {
	t.Parallel()
	if err := (serveFlags{playbookPath: "site.playbook.yaml"}).validateSources(); err == nil || !strings.Contains(err.Error(), "--playbook requires --spec") {
		t.Fatalf("validate a playbook without a spec = %v, want it refused", err)
	}
}

func TestLoadPlaybookIsOptionalAndCompilesAgainstTheSpec(t *testing.T) {
	t.Parallel()
	compiled := compilePumpStation(t)
	if playbook, err := loadPlaybook(compiled, ""); playbook != nil || err != nil {
		t.Fatalf("no playbook path = %v, %v; want none", playbook, err)
	}
	playbook, err := loadPlaybook(compiled, "../../examples/pump-station/testdata/site-a.playbook.yaml")
	if err != nil || len(playbook.Rules) != 2 {
		t.Fatalf("load the pump playbook = %v, %v", playbook, err)
	}
	if _, err := loadPlaybook(compiled, "missing.playbook.yaml"); err == nil || !strings.Contains(err.Error(), "compile playbook") {
		t.Fatalf("a missing playbook = %v, want it refused", err)
	}
}
