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
	mine, err := filepath.Glob(filepath.Join(conveyorKinds, "*.kind.json"))
	if err != nil || len(mine) != 5 {
		t.Fatalf("kind files in %s = %v (%v), want the feeder, the belt, the drive and its master and follower roles", conveyorKinds, mine, err)
	}
	for _, path := range mine {
		name := filepath.Base(path)
		if !bytes.Equal(readKindBytes(t, path), readMasterKind(t, root, name)) {
			t.Errorf("%s differs from %s: copy it again", path, filepath.Join(master, name))
		}
	}
}
