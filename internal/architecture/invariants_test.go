package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	invariantsPath  = "documentation/architecture/invariants.md"
	invariantsCount = 10
)

var (
	invariantHeading = regexp.MustCompile(`^### Invariant (\d+):`)
	provingTestLink  = regexp.MustCompile("\\[`(Test\\w+)`\\]\\(([^)]+)\\)")
)

type provingTest struct {
	name string
	link string
}

// TestEveryInvariantNamesTestsThatExist enforces test bar rule T12: each of the
// ten product invariants lists proving tests in the invariants page, and every
// listed test is a function in the file its link points to.
func TestEveryInvariantNamesTestsThatExist(t *testing.T) {
	t.Parallel()

	root := loadRepository(t).root
	content, err := os.ReadFile(filepath.Join(root, invariantsPath))
	if err != nil {
		t.Fatalf("read invariants page: %v", err)
	}
	proofs := provingTests(string(content))
	for number := 1; number <= invariantsCount; number++ {
		if len(proofs[number]) == 0 {
			t.Errorf("invariant %d lists no proving test in %s", number, invariantsPath)
		}
	}
	for number, tests := range proofs {
		for _, proof := range tests {
			assertTestDeclared(t, filepath.Join(root, filepath.Dir(invariantsPath)), number, proof)
		}
	}
}

func TestProvingTestsAreReadPerInvariantSection(t *testing.T) {
	t.Parallel()
	page := "## Proving tests\n\n### Invariant 1: A.\n\n- `m`: [`TestA`](../a_test.go), [`TestB`](../b_test.go)\n\n" +
		"### Invariant 2: B.\n\n- `m`: [`TestC`](../c_test.go)\n"
	got := provingTests(page)
	want := map[int][]provingTest{
		1: {{"TestA", "../a_test.go"}, {"TestB", "../b_test.go"}},
		2: {{"TestC", "../c_test.go"}},
	}
	if len(got) != len(want) || !slices.Equal(got[1], want[1]) || !slices.Equal(got[2], want[2]) {
		t.Fatalf("provingTests = %v, want %v", got, want)
	}
}

func provingTests(page string) map[int][]provingTest {
	proofs := map[int][]provingTest{}
	current := 0
	for line := range strings.SplitSeq(page, "\n") {
		if heading := invariantHeading.FindStringSubmatch(line); heading != nil {
			current, _ = strconv.Atoi(heading[1])
			continue
		}
		for _, match := range provingTestLink.FindAllStringSubmatch(line, -1) {
			proofs[current] = append(proofs[current], provingTest{name: match[1], link: match[2]})
		}
	}
	return proofs
}

func assertTestDeclared(t *testing.T, directory string, invariant int, proof provingTest) {
	t.Helper()

	source, err := os.ReadFile(filepath.Join(directory, proof.link))
	if err != nil {
		t.Errorf("invariant %d: %s links to %s: %v", invariant, proof.name, proof.link, err)
		return
	}
	if !strings.Contains(string(source), "func "+proof.name+"(") {
		t.Errorf("invariant %d: %s is not declared in %s; rename the entry in %s", invariant, proof.name, proof.link, invariantsPath)
	}
}
