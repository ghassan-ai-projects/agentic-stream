package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestLogFlagsInstallTheRequestedHandler(t *testing.T) { //nolint:paralleltest // It replaces the process-wide default logger.
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var out bytes.Buffer
	flags := logFlags{level: "warn", format: "json"}
	if err := flags.install(&out); err != nil {
		t.Fatal(err)
	}
	slog.Info("hidden")
	slog.Warn("shown", "epoch", "e1")
	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil || line["msg"] != "shown" || line["epoch"] != "e1" {
		t.Fatalf("log output = %q err=%v, want one JSON warning", out.String(), err)
	}
}

func TestLogFlagsRefuseUnknownValues(t *testing.T) {
	t.Parallel()
	for _, flags := range []logFlags{{level: "loud", format: "text"}, {level: "info", format: "xml"}} {
		if err := flags.install(&bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--log-") {
			t.Fatalf("flags %+v were accepted", flags)
		}
	}
}
