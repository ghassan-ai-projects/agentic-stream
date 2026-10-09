package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const kernelPackage = "internal/kernel"

var kernelForbiddenImports = []string{
	"database/sql", "os", "os/exec", "os/signal", "os/user", "io/fs", "io/ioutil", "path/filepath",
	"net", "net/http", "syscall", "plugin",
	"math/rand", "math/rand/v2", "crypto/rand", "hash/maphash",
}

var kernelForbiddenTimeSelectors = []string{
	"Now", "Since", "Until", "Sleep", "After", "Tick", "NewTimer", "NewTicker", "AfterFunc",
	"Local", "LoadLocation",
}

// TestKernelStaysPure keeps the shared kernel importable by every package: it
// imports only the standard library, no database, file, network or random
// source, and never reads the wall clock or the host time zone.
func TestKernelStaysPure(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).filesIn(t, kernelPackage) {
		for _, violation := range kernelViolations(file.syntax) {
			t.Errorf("%s: %s", file.rel, violation)
		}
	}
}

func TestKernelPurityCheckCatchesEveryBypass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{"clean code", "package k\nimport (\"fmt\"\n\"time\")\nfunc f(t time.Time) string { return fmt.Sprint(t.UTC(), time.UTC, time.Second) }", nil},
		{"direct clock read", "package k\nimport \"time\"\nvar n = time.Now()", []string{"time.Now"}},
		{"aliased time import", "package k\nimport tm \"time\"\nvar n = tm.Now()", []string{"tm.Now"}},
		{"alias hides a literal time identifier", "package k\nimport tm \"time\"\nvar time = struct{ Now int }{}\nvar n = time.Now\nvar u = tm.Until", []string{"tm.Until"}},
		{"dot import of time", "package k\nimport . \"time\"\nvar n = Now()", []string{"dot-imports time"}},
		{"dot import of any package", "package k\nimport . \"strings\"\nvar n = ToUpper(\"a\")", []string{"dot-imports strings"}},
		{"blank import of a forbidden package", "package k\nimport _ \"os\"", []string{"imports os"}},
		{"host time zone", "package k\nimport \"time\"\nvar z = time.Local", []string{"time.Local"}},
		{"time zone database read", "package k\nimport \"time\"\nvar z, _ = time.LoadLocation(\"UTC\")", []string{"time.LoadLocation"}},
		{"timers", "package k\nimport \"time\"\nvar a, b, c = time.After, time.NewTicker, time.AfterFunc", []string{"time.After", "time.NewTicker", "time.AfterFunc"}},
		{"log package is allowed", "package k\nimport (\"log\"\n\"log/slog\"\n\"runtime\"\n\"unsafe\")\nvar _ = log.Println\nvar _ = slog.Info\nvar _ = runtime.GOOS\nvar _ unsafe.Pointer", nil},
		{"signal", "package k\nimport \"os/signal\"", []string{"imports os/signal"}},
		{"filepath", "package k\nimport \"path/filepath\"", []string{"imports path/filepath"}},
		{"maphash", "package k\nimport \"hash/maphash\"", []string{"imports hash/maphash"}},
		{"user", "package k\nimport \"os/user\"", []string{"imports os/user"}},
		{"plugin", "package k\nimport \"plugin\"", []string{"imports plugin"}},
		{"module package", "package k\nimport \"github.com/ghassan-ai-projects/agentic-stream/internal/sources\"", []string{"standard library"}},
		{"repository package", "package k\nimport \"internal/sources\"", []string{"standard library"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "snippet.go", test.source, 0)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			got := kernelViolations(parsed)
			if len(got) != len(test.want) {
				t.Fatalf("violations = %q, want %d mentioning %q", got, len(test.want), test.want)
			}
			for _, want := range test.want {
				if !slices.ContainsFunc(got, func(violation string) bool { return strings.Contains(violation, want) }) {
					t.Errorf("violations = %q, want one mentioning %q", got, want)
				}
			}
		})
	}
}

func kernelViolations(parsed *ast.File) []string {
	violations := kernelImportViolations(parsed)
	return append(violations, kernelClockViolations(parsed)...)
}

func kernelImportViolations(parsed *ast.File) []string {
	var violations []string
	for _, spec := range parsed.Imports {
		imported, _ := strconv.Unquote(spec.Path.Value)
		if spec.Name != nil && spec.Name.Name == "." {
			violations = append(violations, "kernel dot-imports "+imported+"; name every symbol through its package")
		}
		if strings.Contains(strings.SplitN(imported, "/", 2)[0], ".") || strings.HasPrefix(imported, "internal/") {
			violations = append(violations, "kernel imports "+imported+"; the kernel imports only the standard library")
		}
		if slices.Contains(kernelForbiddenImports, imported) {
			violations = append(violations, "kernel imports "+imported+", which reaches outside pure computation")
		}
	}
	return violations
}

func kernelClockViolations(parsed *ast.File) []string {
	names := kernelTimeNames(parsed)
	var violations []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := selector.X.(*ast.Ident); ok && names[ident.Name] && slices.Contains(kernelForbiddenTimeSelectors, selector.Sel.Name) {
			violations = append(violations, "kernel uses "+ident.Name+"."+selector.Sel.Name+"; pass the instant in")
		}
		return true
	})
	return violations
}

func kernelTimeNames(parsed *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, spec := range parsed.Imports {
		if imported, _ := strconv.Unquote(spec.Path.Value); imported != "time" {
			continue
		}
		if spec.Name == nil {
			names["time"] = true
		} else {
			names[spec.Name.Name] = true
		}
	}
	return names
}

// TestKernelHasNoSubpackages keeps the kernel one small package: a subpackage
// would be a second layer with its own rules.
func TestKernelHasNoSubpackages(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).filesUnder(kernelPackage) {
		if file.dir != kernelPackage {
			t.Errorf("%s: the kernel is a single package", file.rel)
		}
	}
}

// TestDigestTextHasOneOwner keeps the "sha256:<hex>" text form in
// internal/kernel: no other production package concatenates the prefix onto a
// hex sum; it calls kernel.EncodeDigest (canonicaljson.EncodeDigest delegates).
func TestDigestTextHasOneOwner(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).production {
		if strings.HasPrefix(file.rel, kernelPackage+"/") {
			continue
		}
		ast.Inspect(file.syntax, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING && literal.Value == `"sha256:"` {
				t.Errorf("%s: spell digests with kernel.EncodeDigest and DecodeDigest, not the bare prefix", file.rel)
			}
			return true
		})
	}
}
