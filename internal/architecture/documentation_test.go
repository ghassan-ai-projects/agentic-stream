package architecture

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// moduleMapPath is the public module map that architecture-bar rule A9
// requires to list every production package.
const moduleMapPath = "documentation/architecture/repository-map.md"

// TestEveryPackageDocumentsItsResponsibility enforces architecture-bar rule
// A9: each production package names its responsibility in a package comment.
func TestEveryPackageDocumentsItsResponsibility(t *testing.T) {
	t.Parallel()

	documented := documentedPackages(loadRepository(t))
	for _, pkg := range slices.Sorted(maps.Keys(documented)) {
		if !documented[pkg] {
			t.Errorf("%s has no package comment; state its business or infrastructure responsibility in doc.go", pkg)
		}
	}
}

// TestModuleMapListsEveryPackage enforces architecture-bar rule A9: the public
// module map names every production package.
func TestModuleMapListsEveryPackage(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	moduleMap := readModuleMap(t, repo.root)
	for _, pkg := range slices.Sorted(maps.Keys(documentedPackages(repo))) {
		if !strings.Contains(moduleMap, "`"+pkg+"/`") {
			t.Errorf("%s is missing from %s", pkg, moduleMapPath)
		}
	}
}

// documentedPackages maps each production package directory to whether any
// hand-written file carries a "Package <name>" comment. Generated-only
// packages are exempt from the comment rule and reported as documented.
func documentedPackages(repo *repository) map[string]bool {
	documented := map[string]bool{}
	for _, file := range repo.production {
		if file.dir == "." || strings.HasPrefix(file.dir, "examples/") {
			continue
		}
		documented[file.dir] = documented[file.dir] || fileDocumentsPackage(file)
	}
	return documented
}

// fileDocumentsPackage reports whether the file is generated or carries the
// package comment.
func fileDocumentsPackage(file goFile) bool {
	if file.generated {
		return true
	}
	doc := file.syntax.Doc
	return doc != nil && strings.HasPrefix(doc.Text(), "Package "+file.syntax.Name.Name+" ")
}

func readModuleMap(t *testing.T, root string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, moduleMapPath))
	if err != nil {
		t.Fatalf("read module map: %v", err)
	}
	return string(content)
}
