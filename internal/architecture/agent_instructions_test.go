package architecture

import (
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// agentInstructionsLineBudget caps AGENTS.md, which every agent loads on every
// task, at Anthropic's guidance of under 200 lines. Rules needed while writing
// code stay in the file; procedures belong in .agents/context/.
const agentInstructionsLineBudget = 199

var (
	instructionLink       = regexp.MustCompile(`\]\(([^)#\s]+)(?:#[^)]*)?\)`)
	instructionImport     = regexp.MustCompile(`(?m)(?:^|\s)@(\.{0,2}/?[\w./-]+\.md)`)
	instructionTestName   = regexp.MustCompile(`\bTest[A-Z][A-Za-z0-9]+\b`)
	instructionMakeTarget = regexp.MustCompile("`make ([a-z][a-z-]*)")
	instructionFilePath   = regexp.MustCompile("`([\\w.-]+(?:/[\\w.-]+)+\\.[a-z]+)`")
)

// instructionFacts is what the instruction files may refer to.
type instructionFacts struct {
	tests   map[string]bool
	targets map[string]bool
	exists  func(path string) bool
}

// TestAgentInstructionsStayShortAndCurrent keeps the always-loaded file within
// its budget and every reference in the instruction files alive, so stale
// guidance fails CI instead of misleading the next agent.
func TestAgentInstructionsStayShortAndCurrent(t *testing.T) {
	t.Parallel()

	root := loadRepository(t).root
	agents := readInstructionFile(t, filepath.Join(root, "AGENTS.md"))
	if lines := strings.Count(agents, "\n"); lines > agentInstructionsLineBudget {
		t.Errorf("AGENTS.md has %d lines, budget %d; move detail to .agents/context/", lines, agentInstructionsLineBudget)
	}
	facts := currentInstructionFacts(t, root)
	for _, file := range instructionFiles(t, root) {
		for _, problem := range staleInstructionReferences(root, file, readInstructionFile(t, file), facts) {
			t.Errorf("%s: %s", relativeToRoot(root, file), problem)
		}
	}
}

// TestStaleInstructionCheckCatchesEveryKindOfReference is the gate's self-test.
func TestStaleInstructionCheckCatchesEveryKindOfReference(t *testing.T) {
	t.Parallel()

	facts := instructionFacts{
		tests:   map[string]bool{"TestKnown": true},
		targets: map[string]bool{"build": true},
		exists: func(path string) bool {
			return strings.HasSuffix(path, "real.md") || strings.HasSuffix(path, "real.go")
		},
	}
	cases := []struct {
		name, text string
		want       int
	}{
		{"live references pass", "[a](real.md) `make build` TestKnown `pkg/real.go` @real.md", 0},
		{"dead link", "[a](gone.md)", 1},
		{"dead import", "see @../gone.md", 1},
		{"unknown test", "TestGone", 1},
		{"unknown make target", "`make gone`", 1},
		{"unknown file path", "`pkg/gone.go`", 1},
		{"external link and glob are skipped", "[a](https://example.com) `pkg/*.go`", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := staleInstructionReferences("/repo", "/repo/AGENTS.md", tc.text, facts)
			if len(got) != tc.want {
				t.Errorf("got %d problems %v, want %d", len(got), got, tc.want)
			}
		})
	}
}

func instructionFiles(t *testing.T, root string) []string {
	t.Helper()

	files := []string{
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "CLAUDE.md"),
		filepath.Join(root, "GEMINI.md"),
		filepath.Join(root, ".github/copilot-instructions.md"),
	}
	for _, dir := range []string{".agents/context", ".agents/prompts", ".claude/rules"} {
		matches, err := filepath.Glob(filepath.Join(root, dir, "*.md"))
		if err != nil {
			t.Fatalf("list %s: %v", dir, err)
		}
		files = append(files, matches...)
	}
	return files
}

func readInstructionFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instruction file: %v", err)
	}
	return string(content)
}

func currentInstructionFacts(t *testing.T, root string) instructionFacts {
	t.Helper()

	return instructionFacts{
		tests:   declaredTestNames(t),
		targets: declaredMakeTargets(readInstructionFile(t, filepath.Join(root, "Makefile"))),
		exists:  pathExists,
	}
}

func declaredTestNames(t *testing.T) map[string]bool {
	t.Helper()

	tests, err := loadRepository(t).tests()
	if err != nil {
		t.Fatalf("load test files: %v", err)
	}
	names := map[string]bool{}
	for _, file := range tests {
		for _, decl := range file.syntax.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				names[fn.Name.Name] = true
			}
		}
	}
	return names
}

func declaredMakeTargets(makefile string) map[string]bool {
	targets := map[string]bool{}
	for _, line := range strings.Split(makefile, "\n") {
		if name, _, found := strings.Cut(line, ":"); found && !strings.ContainsAny(name, " \t#=$") {
			targets[name] = true
		}
	}
	return targets
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// staleInstructionReferences lists references in one instruction file's text
// that no longer resolve. Links and imports resolve from the file itself;
// backticked paths resolve from the repository root.
func staleInstructionReferences(root, file, text string, facts instructionFacts) []string {
	seen := map[string]bool{}
	for _, target := range relativeTargets(text) {
		if !facts.exists(filepath.Join(filepath.Dir(file), target)) {
			seen["dead link or import "+target] = true
		}
	}
	for _, name := range instructionTestName.FindAllString(text, -1) {
		if !facts.tests[name] {
			seen["unknown test "+name] = true
		}
	}
	for _, match := range instructionMakeTarget.FindAllStringSubmatch(text, -1) {
		if !facts.targets[match[1]] {
			seen["unknown make target "+match[1]] = true
		}
	}
	for _, match := range instructionFilePath.FindAllStringSubmatch(text, -1) {
		if !facts.exists(filepath.Join(root, match[1])) {
			seen["unknown file path "+match[1]] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func relativeTargets(text string) []string {
	var targets []string
	for _, pattern := range []*regexp.Regexp{instructionLink, instructionImport} {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			if !strings.Contains(match[1], "://") {
				targets = append(targets, match[1])
			}
		}
	}
	return targets
}

func relativeToRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
