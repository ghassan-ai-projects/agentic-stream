package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"testing"
)

// sleepAllowlist names the test files that may call time.Sleep, each with the
// reason a condition, channel or virtual clock cannot replace the wait. Test
// bar rule T5: a test never sleeps to wait for work. A new entry is reviewed
// with the test that needs it.
var sleepAllowlist = map[string]string{}

// TestTestsNeverSleep enforces test bar rule T5: no _test.go file calls
// time.Sleep, so a test waits on a channel, a condition or a virtual clock.
func TestTestsNeverSleep(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	tests, err := repo.tests()
	if err != nil {
		t.Fatalf("parse test files: %v", err)
	}
	for _, file := range tests {
		if _, allowed := sleepAllowlist[file.rel]; allowed {
			continue
		}
		for _, call := range sleepCalls(file.syntax) {
			t.Errorf("%s: time.Sleep; wait on a channel, a condition bound to t.Context() or a virtual clock", repo.position(call))
		}
	}
}

// TestSleepAllowlistHasNoStaleEntries keeps the allowlist honest: an entry
// whose file no longer sleeps is removed.
func TestSleepAllowlistHasNoStaleEntries(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	tests, err := repo.tests()
	if err != nil {
		t.Fatalf("parse test files: %v", err)
	}
	sleeping := map[string]bool{}
	for _, file := range tests {
		sleeping[file.rel] = len(sleepCalls(file.syntax)) > 0
	}
	for _, path := range slices.Sorted(maps.Keys(sleepAllowlist)) {
		if !sleeping[path] {
			t.Errorf("%s is allowed to call time.Sleep but does not; remove the entry", path)
		}
	}
}

func TestSleepDetectorCatchesEveryWayToCallTimeSleep(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{"direct call", "package p\nimport \"time\"\nfunc f() { time.Sleep(time.Second) }", 1},
		{"aliased import", "package p\nimport tm \"time\"\nfunc f() { tm.Sleep(1) }", 1},
		{"function value", "package p\nimport \"time\"\nvar wait = time.Sleep", 1},
		{"two calls", "package p\nimport \"time\"\nfunc f() { time.Sleep(1); time.Sleep(2) }", 2},
		{"timers and durations are not sleeps", "package p\nimport \"time\"\nfunc f() { _ = time.NewTimer(time.Second); _ = time.Now() }", 0},
		{"a Sleep method of another package", "package p\nimport \"example.com/clock\"\nfunc f() { clock.Sleep(1) }", 0},
		{"an alias hides a literal time identifier", "package p\nimport tm \"time\"\nvar time = struct{ Sleep func(int) }{}\nfunc f() { time.Sleep(1); tm.Sleep(1) }", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "snippet_test.go", test.source, 0)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			if got := len(sleepCalls(parsed)); got != test.want {
				t.Fatalf("time.Sleep uses = %d, want %d", got, test.want)
			}
		})
	}
}

// sleepCalls lists every use of time.Sleep in a file, whatever name the file
// gives the time package.
func sleepCalls(parsed *ast.File) []ast.Node {
	names := timeImportNames(parsed)
	var uses []ast.Node
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Sleep" {
			return true
		}
		if ident, ok := selector.X.(*ast.Ident); ok && names[ident.Name] {
			uses = append(uses, selector)
		}
		return true
	})
	return uses
}
