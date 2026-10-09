package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNothingExecutableReadsFromTheDocsArchive enforces that docs/ stays a dated
// working archive: no Go file, tests included, names a path under docs/, and
// the Makefile uses nothing there as an input. Test inputs live in examples/
// (fixtures shared by modules) or in the owning module's testdata/.
func TestNothingExecutableReadsFromTheDocsArchive(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.allFiles(t) {
		if isArchiveFile(file) {
			continue
		}
		for _, literal := range docsPathLiterals(file.syntax) {
			t.Errorf("%s: %s names a path under docs/; keep test inputs in examples/ or the module's testdata/", repo.position(literal), literal.Value)
		}
	}
}

func TestMakefileDoesNotUseTheDocsArchive(t *testing.T) {
	t.Parallel()

	text, err := os.ReadFile(filepath.Join(loadRepository(t).root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	for _, line := range makefileDocsReads(string(text)) {
		t.Errorf("Makefile %s; the build must not use docs/ as an input", line)
	}
}

func TestDocsArchiveDetectorCatchesEveryWayToNameAPathUnderDocs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{"relative path", `var p = "docs/design/x.yaml"`, 1},
		{"one parent", `var p = "../docs/design/x.yaml"`, 1},
		{"many parents", `var p = "../../../../docs/design/x.yaml"`, 1},
		{"current directory prefix", `var p = "./docs/x"`, 1},
		{"path as a prefix of a concatenation", `var p = "../../docs/design/examples/" + name`, 1},
		{"raw string", "var p = `docs/design/x.yaml`", 1},
		{"join with docs as an element", `var p = filepath.Join("..", "..", "docs", "design", "x.yaml")`, 1},
		{"join after a variable root", `var p = filepath.Join(root, "docs", "x")`, 1},
		{"path join", `var p = path.Join("docs", "x")`, 1},
		{"join with a slash path", `var p = filepath.Join(root, "docs/x")`, 1},
		{"line comment", "// reads docs/design/x.yaml\nvar p = 1", 0},
		{"block comment", "/* docs/design/x.yaml */\nvar p = 1", 0},
		{"docs inside a message", `var m = "see docs/design/x.yaml for the pin"`, 0},
		{"documentation directory", `var p = "documentation/guides/x.md"`, 0},
		{"examples directory", `var p = "../../examples/predictive-maintenance/x.yaml"`, 0},
		{"docs as a name outside a join", `var kind = "docs"`, 0},
		{"a longer first element", `var p = "docs-archive/x"`, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "snippet_test.go", "package p\n"+test.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			if got := len(docsPathLiterals(parsed)); got != test.want {
				t.Fatalf("docs paths found = %d, want %d", got, test.want)
			}
		})
	}
}

func TestMakefileDetectorReadsRecipesAndVariablesButNotComments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"variable", "PROTO := docs/design/x.proto\n", []string{"line 1: PROTO := docs/design/x.proto"}},
		{"recipe", "build:\n\tprotoc --proto_path=docs/design x.proto\n", []string{"line 2: protoc --proto_path=docs/design x.proto"}},
		{"comment line", "# docs/design/x.proto is archived\n", nil},
		{"trailing comment", "build: # see docs/design\n\tgo build\n", nil},
		{"target named docs-check", "docs-check:\n\tpython3 scripts/check-documentation.py\n", nil},
		{"documentation directory", "\tcheck documentation/guides\n", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := makefileDocsReads(test.text)
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("reads = %q, want %q", got, test.want)
			}
		})
	}
}

const docsDirectory = "docs"

// isArchiveFile reports whether a Go file belongs to the archive itself: the
// frozen design-v0.1 reference module, its own Go module that no build builds.
func isArchiveFile(file goFile) bool {
	return strings.HasPrefix(file.rel, docsDirectory+"/")
}

// docsPathLiterals lists the string literals of a file that name a path under
// the docs directory at the repository root: a literal that starts with docs/
// (after any ./ or ../ prefix), or a docs element of a Join call. Comments and
// messages that merely mention docs/ are not paths.
func docsPathLiterals(parsed *ast.File) []*ast.BasicLit {
	var found []*ast.BasicLit
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.BasicLit:
			if startsAtDocs(node) {
				found = append(found, node)
			}
		case *ast.CallExpr:
			found = append(found, docsJoinElements(node)...)
		}
		return true
	})
	return found
}

func startsAtDocs(literal *ast.BasicLit) bool {
	value, ok := stringValue(literal)
	if !ok {
		return false
	}
	return strings.HasPrefix(trimRelativePrefix(value), docsDirectory+"/")
}

func docsJoinElements(call *ast.CallExpr) []*ast.BasicLit {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Join" {
		return nil
	}
	var found []*ast.BasicLit
	for _, argument := range call.Args {
		literal, ok := argument.(*ast.BasicLit)
		if !ok {
			continue
		}
		if value, ok := stringValue(literal); ok && trimRelativePrefix(value) == docsDirectory {
			found = append(found, literal)
		}
	}
	return found
}

func stringValue(literal *ast.BasicLit) (string, bool) {
	if literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func trimRelativePrefix(value string) string {
	for {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(value, "./"), "../"), "/")
		if trimmed == value {
			return value
		}
		value = trimmed
	}
}

// makefileDocsReads lists the lines of a Makefile whose code, comments
// excluded, contains a docs/ path.
func makefileDocsReads(text string) []string {
	var reads []string
	for number, line := range strings.Split(text, "\n") {
		code, _, _ := strings.Cut(line, "#")
		if strings.Contains(code, docsDirectory+"/") {
			reads = append(reads, fmt.Sprintf("line %d: %s", number+1, strings.TrimSpace(code)))
		}
	}
	return reads
}
