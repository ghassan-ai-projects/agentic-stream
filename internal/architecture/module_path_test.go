package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModulePathSingleSourceOfTruth pins the module path declared in go.mod to
// the few places that must repeat it verbatim because no tool can interpolate
// go.mod:
//
//   - the OpenTelemetry instrumentation scope constant
//   - the protobuf go_package option
//   - the goimports -local prefix used by pre-commit
//
// Import statements are deliberately not checked: every internal import names
// the module in full because the Go toolchain requires a module-qualified,
// non-relative import path.
//
// A module rename that misses one of these copies fails here instead of
// silently shipping a stale identity. See .agents/context/go-style.md.
func TestModulePathSingleSourceOfTruth(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	root, module := repo.root, repo.module

	cases := []struct {
		file string
		want string
	}{
		{
			file: "internal/telemetry/internal/transport/otel.go",
			want: `const InstrumentationName = "` + module + `"`,
		},
		{
			file: "docs/design/contracts/runtime-v1.proto",
			want: `option go_package = "` + module + `/proto/agenticstream/runtime/v1;runtimev1";`,
		},
		{
			file: ".pre-commit-config.yaml",
			want: "-local, " + module,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.file, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join(root, testCase.file))
			if err != nil {
				t.Fatalf("read %s: %v", testCase.file, err)
			}
			if !strings.Contains(string(body), testCase.want) {
				t.Errorf("%s does not carry the go.mod module path; want %q", testCase.file, testCase.want)
			}
		})
	}

	// Build tooling has no reason to hardcode the path: it can ask the Go
	// toolchain for it.
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(makefile), "MODULE    ?= $(shell go list -m") {
		t.Error("Makefile must derive MODULE from go.mod via `go list -m`")
	}
}
