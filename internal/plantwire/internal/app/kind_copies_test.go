package app_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const streamsimRepositoryVariable = "STREAMSIM_REPO"

func TestTheBenchKindFilesAreByteCopiesOfStreamsims(t *testing.T) {
	t.Parallel()
	master := filepath.Join(cmp.Or(os.Getenv(streamsimRepositoryVariable), filepath.Join("..", "..", "..", "..", "..", "streams-simulator")),
		"examples", "devices", "thermal-board")
	root, err := os.OpenRoot(master)
	if err != nil {
		t.Skipf("no streams-simulator checkout beside this one or in %s: %v", streamsimRepositoryVariable, err)
	}
	defer func() { _ = root.Close() }()
	copies, err := filepath.Glob(filepath.Join(benchExample, "kinds", "*.kind.json"))
	if err != nil || len(copies) == 0 {
		t.Fatalf("no kind file copies in %s: %v", benchExample, err)
	}
	for _, copied := range copies {
		mine, theirs := readKindBytes(t, copied), readMasterKind(t, root, filepath.Base(copied))
		if bytes.Equal(mine, theirs) {
			continue
		}
		if !slices.Equal(commandNames(t, mine), commandNames(t, theirs)) {
			t.Logf("%s in %s announces other commands (%v, here %v): streamsim does not hold the semantic kinds yet",
				filepath.Base(copied), master, commandNames(t, theirs), commandNames(t, mine))
			continue
		}
		t.Errorf("%s differs from %s: copy it again", copied, filepath.Join(master, filepath.Base(copied)))
	}
}

func readKindBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func readMasterKind(t *testing.T, root *os.Root, name string) []byte {
	t.Helper()
	raw, err := root.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func commandNames(t *testing.T, raw []byte) []string {
	t.Helper()
	var kind struct {
		Commands []struct {
			Name string `json:"name"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &kind); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, command := range kind.Commands {
		names = append(names, command.Name)
	}
	slices.Sort(names)
	return names
}
