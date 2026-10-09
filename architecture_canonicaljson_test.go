package agenticstream

import (
	"go/ast"
	"go/token"
	"path"
	"strings"
	"testing"
)

// TestCanonicalJSONFacadeOnlyDelegates confines the encoder, validator and digest rules to the domain layer.
func TestCanonicalJSONFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/canonicaljson" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && !facadeDelegation(function, "domain") {
				t.Errorf("%s: %s must only delegate through the canonical JSON facade", file.rel, function.Name)
			}
		}
	}
}

// TestDigestTextHasOneOwner keeps the "sha256:<hex>" text form in
// internal/kernel: no other production package concatenates the prefix onto a
// hex sum; it calls kernel.EncodeDigest (canonicaljson.EncodeDigest delegates).
func TestDigestTextHasOneOwner(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if strings.HasPrefix(file.rel, "internal/kernel/") {
			continue
		}
		parsed := parseGoFile(t, file)
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING && literal.Value == `"sha256:"` {
				t.Errorf("%s: spell digests with kernel.EncodeDigest and DecodeDigest, not the bare prefix", file.rel)
			}
			return true
		})
	}
}
