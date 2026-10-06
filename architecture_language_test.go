package agenticstream

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEveryLayeredModuleHasItsUbiquitousLanguage requires the vocabulary of a
// module to live beside its code, so the next change reads it first.
func TestEveryLayeredModuleHasItsUbiquitousLanguage(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	modules, err := filepath.Glob(filepath.Join(root, "internal", "*", "internal", "domain"))
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range modules {
		module := filepath.Dir(filepath.Dir(domain))
		if _, err := os.Stat(filepath.Join(module, "UBIQUITOUS_LANGUAGE.md")); err != nil {
			rel, _ := filepath.Rel(root, module)
			t.Errorf("%s has a domain layer but no UBIQUITOUS_LANGUAGE.md", filepath.ToSlash(rel))
		}
	}
}
