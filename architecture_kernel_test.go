package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"strings"
	"testing"
)

const kernelPackage = "internal/kernel"

var kernelForbiddenImports = []string{
	"database/sql", "os", "os/exec", "io/fs", "io/ioutil", "net", "net/http", "syscall",
	"math/rand", "math/rand/v2", "crypto/rand",
}

var kernelForbiddenTimeCalls = []string{"Now", "Since", "Until", "Sleep", "After", "Tick", "NewTimer", "NewTicker", "AfterFunc"}

// TestKernelStaysPure keeps the shared kernel importable by every package: it
// imports only the standard library, no database, file, network or random
// source, and never reads the wall clock.
func TestKernelStaysPure(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	found := false
	for _, file := range productionGoFiles(t, root) {
		if path.Dir(file.rel) != kernelPackage {
			continue
		}
		found = true
		parsed := parseGoFile(t, file)
		for _, imported := range importPaths(t, file, parsed) {
			if strings.Contains(strings.SplitN(imported, "/", 2)[0], ".") || strings.HasPrefix(imported, "internal/") {
				t.Errorf("%s: kernel imports %s; the kernel imports only the standard library", file.rel, imported)
			}
			if slices.Contains(kernelForbiddenImports, imported) {
				t.Errorf("%s: kernel imports %s, which reaches outside pure computation", file.rel, imported)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == "time" && slices.Contains(kernelForbiddenTimeCalls, selector.Sel.Name) {
				t.Errorf("%s: kernel calls time.%s; pass the instant in", file.rel, selector.Sel.Name)
			}
			return true
		})
	}
	if !found {
		t.Fatalf("no production files found in %s", kernelPackage)
	}
}

// TestKernelHasNoSubpackages keeps the kernel one small package: a subpackage
// would be a second layer with its own rules.
func TestKernelHasNoSubpackages(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if strings.HasPrefix(file.rel, kernelPackage+"/") && path.Dir(file.rel) != kernelPackage {
			t.Errorf("%s: the kernel is a single package", file.rel)
		}
	}
}
