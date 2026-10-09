package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// The real-world-sensor runbooks (RUNBOOK-G1, RUNBOOK-HIL-THIS-WEEK) start the
// runtime with these commands and flags. Renaming or removing one breaks the
// experiment without failing anything else in this repository; new commands
// and flags are additive.
func TestExperimentCommandsKeepTheirFlags(t *testing.T) {
	t.Parallel()
	commands := map[string]*cobra.Command{}
	for _, command := range newRootCommand().Commands() {
		commands[command.Name()] = command
	}
	required := map[string][]string{
		"validate": {"json"},
		"serve": {
			"db", "spec", "tenant", "live-socket", "trace-format", "worker-socket", "worker-name",
			"effect-profile", "device-socket", "device-catalog", "device-firmware-digest",
			"live-actuation", "owner-authorized", "owner-lease", "listen", "poll-interval",
		},
		"run-live":   {"db", "spec", "trace", "trace-format", "effect-profile"},
		"export-run": {"db", "tenant", "output"},
		"verify-run": {},
	}
	for name, flags := range required {
		command, ok := commands[name]
		if !ok {
			t.Errorf("command %q is gone; the real-world-sensor runbooks call it", name)
			continue
		}
		for _, flag := range flags {
			if command.Flags().Lookup(flag) == nil {
				t.Errorf("%s --%s is gone; the real-world-sensor runbooks pass it", name, flag)
			}
		}
	}
}
