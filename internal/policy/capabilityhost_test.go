package policy_test

import (
	"context"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestCapabilityHostIsExplicitAllowlist(t *testing.T) {
	host, err := policy.NewCapabilityHost([]policy.Capability{{
		Name: "evidence_get",
		Call: func(context.Context, map[string]any) (map[string]any, error) { return map[string]any{"ok": true}, nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := host.Call(context.Background(), "evidence_get", nil)
	if err != nil || result["ok"] != true {
		t.Fatalf("allowlisted capability result=%v err=%v", result, err)
	}
	if _, err := host.Call(context.Background(), "shell", nil); err == nil {
		t.Fatal("unallowlisted capability was callable")
	}
}

func TestCapabilityHostRejectsDuplicateOrIncompleteEntries(t *testing.T) {
	if _, err := policy.NewCapabilityHost([]policy.Capability{{Name: "same", Call: func(context.Context, map[string]any) (map[string]any, error) { return nil, nil }}, {Name: "same", Call: func(context.Context, map[string]any) (map[string]any, error) { return nil, nil }}}); err == nil {
		t.Fatal("duplicate capability was accepted")
	}
	if _, err := policy.NewCapabilityHost([]policy.Capability{{Name: "missing_call"}}); err == nil {
		t.Fatal("incomplete capability was accepted")
	}
}

func TestCapabilityHostDependencyDirection(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate capability host source")
	}
	hostFile := filepath.Join(filepath.Dir(sourceFile), "capabilityhost.go")
	file, err := parser.ParseFile(token.NewFileSet(), hostFile, nil, 0)
	if err != nil {
		t.Fatalf("parse capability host: %v", err)
	}
	forbidden := []string{
		"internal/actions", "internal/storage", "internal/evidence",
		"os", "os/exec", "net", "net/http", "database/sql",
	}
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		for _, prefix := range forbidden {
			if path == prefix || strings.HasSuffix(path, "/"+prefix) {
				t.Fatalf("capability host imports forbidden boundary %q", path)
			}
		}
	}
}

func TestCapabilityHostRejectsInjectionCorpus(t *testing.T) {
	called := false
	host, err := policy.NewCapabilityHost([]policy.Capability{{
		Name: "evidence_get",
		Call: func(context.Context, map[string]any) (map[string]any, error) {
			called = true
			return map[string]any{"read_only": true}, nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"shell", "file_write", "effector", "effect_journal", "mcp", "http",
		"os.exec", "credentials", "unknown",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := host.Call(context.Background(), name, map[string]any{"command": "touch /tmp/x"}); err == nil {
				t.Fatalf("injection capability %q was accepted", name)
			}
		})
	}
	if called {
		t.Fatal("injection corpus reached the evidence capability")
	}
}
