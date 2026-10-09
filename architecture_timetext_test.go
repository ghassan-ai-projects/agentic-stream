package agenticstream

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const storedTimeLayoutPrefix = "2006-01-02T15:04:05"

const timestampGateFile = "architecture_timetext_test.go"

// cloudEventWireFile owns the trimmed RFC 3339 form that the CloudEvents
// envelope digest binds across repositories; it is a contract, not a stored
// timestamp.
const cloudEventWireFile = "internal/contractsv1/internal/domain/cloud_event.go"

// TestTimestampTextHasOneOwner keeps the durable timestamp layout in
// internal/kernel: no other file, test files included, formats or parses an
// instant with time.RFC3339Nano or spells the layout out. The CloudEvents wire
// form is the one named exception.
func TestTimestampTextHasOneOwner(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, file := range allGoFiles(t, root) {
		if strings.HasPrefix(file.rel, "internal/kernel/") || file.rel == timestampGateFile || file.rel == cloudEventWireFile {
			continue
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.abs, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.rel, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if reason := timestampLayoutUse(node); reason != "" {
				t.Errorf("%s: %s: use kernel.FormatTime and kernel.ParseTime", fileSet.Position(node.Pos()), reason)
			}
			return true
		})
	}
}

func timestampLayoutUse(node ast.Node) string {
	switch n := node.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := n.X.(*ast.Ident); ok && pkg.Name == "time" && n.Sel.Name == "RFC3339Nano" {
			return "time.RFC3339Nano names the stored timestamp layout"
		}
	case *ast.BasicLit:
		if value, err := strconv.Unquote(n.Value); err == nil && n.Kind == token.STRING && strings.Contains(value, storedTimeLayoutPrefix) {
			return "a string literal spells out a timestamp layout"
		}
	}
	return ""
}

func allGoFiles(t *testing.T, root string) []goFile {
	t.Helper()
	var files []goFile
	err := filepath.WalkDir(root, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if abs != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "tmp" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		files = append(files, goFile{abs: abs, rel: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	return files
}
