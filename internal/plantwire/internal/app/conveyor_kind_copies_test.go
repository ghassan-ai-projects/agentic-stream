package app_test

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"testing"
)

var conveyorKinds = filepath.Join("..", "..", "..", "..", "examples", "belt-conveyor", "testdata", "plant-wire", "kinds")

func TestTheConveyorKindFilesAreByteCopiesOfStreamsims(t *testing.T) {
	t.Parallel()
	master := filepath.Join(cmp.Or(os.Getenv(streamsimRepositoryVariable), filepath.Join("..", "..", "..", "..", "..", "streams-simulator")),
		"examples", "belt-conveyor", "kinds")
	root, err := os.OpenRoot(master)
	if err != nil {
		t.Skipf("no streams-simulator checkout beside this one or in %s: %v", streamsimRepositoryVariable, err)
	}
	defer func() { _ = root.Close() }()
	theirs, err := filepath.Glob(filepath.Join(master, "*.kind.json"))
	if err != nil || len(theirs) != 3 {
		t.Fatalf("kind files in %s = %v (%v), want the feeder, the belt and the drive", master, theirs, err)
	}
	for _, path := range theirs {
		name := filepath.Base(path)
		if mine := readKindBytes(t, filepath.Join(conveyorKinds, name)); !bytes.Equal(mine, readMasterKind(t, root, name)) {
			t.Errorf("%s differs from %s: copy it again", filepath.Join(conveyorKinds, name), path)
		}
	}
}
