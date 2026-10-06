package agenticstream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryModuleHasItsUbiquitousLanguage requires the vocabulary of every
// module to live beside its code, so the next change reads it first. A module
// is a package directly under internal/ (or internal/executor/) that has
// production Go files; its private layers share the module's vocabulary.
func TestEveryModuleHasItsUbiquitousLanguage(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, pattern := range []string{"internal/*", "internal/executor/*"} {
		modules, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		for _, module := range modules {
			requireLanguage(t, root, module)
		}
	}
}

func requireLanguage(t *testing.T, root, module string) {
	t.Helper()
	if !hasProductionGoFiles(module) {
		return
	}
	if _, err := os.Stat(filepath.Join(module, "UBIQUITOUS_LANGUAGE.md")); err != nil {
		rel, _ := filepath.Rel(root, module)
		t.Errorf("%s has no UBIQUITOUS_LANGUAGE.md", filepath.ToSlash(rel))
	}
}

func hasProductionGoFiles(dir string) bool {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return false
	}
	for _, file := range files {
		if !strings.HasSuffix(file, "_test.go") {
			return true
		}
	}
	return false
}

// moduleShapeExceptions are the stand-alone packages that do not carry a domain
// layer, each with the reason it must not: see
// docs/remaining-migration-2026-10-06/UNIFORM_ARCHITECTURE.md.
var moduleShapeExceptions = map[string]string{
	"internal/interlock":        "SQL-bound port that no contract package or module can host without widening imports",
	"internal/executor/fixture": "demo executor used by composition; held for the owner",
}

// TestEveryModuleHasAFacadeAndADomain requires each module to be a facade over
// internal/domain, so the exposed surface is a deliberate choice and the rules
// have one home. Exceptions are listed above with their reason.
func TestEveryModuleHasAFacadeAndADomain(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, pattern := range []string{"internal/*", "internal/executor/*"} {
		modules, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		for _, module := range modules {
			requireDomain(t, root, module)
		}
	}
}

func requireDomain(t *testing.T, root, module string) {
	t.Helper()
	rel, _ := filepath.Rel(root, module)
	rel = filepath.ToSlash(rel)
	if !hasProductionGoFiles(module) || moduleShapeExceptions[rel] != "" {
		return
	}
	if info, err := os.Stat(filepath.Join(module, "internal", "domain")); err != nil || !info.IsDir() {
		t.Errorf("%s has no internal/domain layer; give it one or list it in moduleShapeExceptions with a reason", rel)
	}
}
