package architecture

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

const storedTimeLayoutPrefix = "2006-01-02T15:04:05"

const timestampGateFile = "internal/architecture/timetext_test.go"

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

	repo := loadRepository(t)
	for _, file := range repo.allFiles(t) {
		if strings.HasPrefix(file.rel, "internal/kernel/") || file.rel == timestampGateFile || file.rel == cloudEventWireFile {
			continue
		}
		ast.Inspect(file.syntax, func(node ast.Node) bool {
			if reason := timestampLayoutUse(node); reason != "" {
				t.Errorf("%s: %s: use kernel.FormatTime and kernel.ParseTime", repo.position(node), reason)
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
