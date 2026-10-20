package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const streamsimBinaryVariable = "STREAMSIM_BIN"

func TestServeHostsTwoSessionsOfTheStreamsimNodeWithoutARestart(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	kinds := filepath.Join(repository, "examples", "devices", "thermal-board")
	run := startPlantWireServe(t, kinds)
	domains := t.TempDir()
	copyFile(t, filepath.Join(kinds, "thermal-board.domain.json"), domains)
	for _, session := range []string{"first", "second"} {
		out := filepath.Join(run.dir, session)
		node := exec.Cmd{Path: binary, Args: []string{binary, "plantwire", "node", "--domain", "arduino-thermal",
			"--domains-dir", domains, "--adapters-dir", filepath.Join(repository, "adapters"), "--kinds-dir", kinds,
			"--host-socket", run.socket, "--duration", "600", "--out", out}}
		if output, err := node.CombinedOutput(); err != nil {
			t.Fatalf("%s session: the node failed: %v\n%s", session, err, output)
		}
		assertEveryBarrierAnswered(t, filepath.Join(out, "journal.jsonl"))
	}
}

func streamsimNode(t *testing.T) (string, string) {
	t.Helper()
	binary := filepath.Clean(os.Getenv(streamsimBinaryVariable))
	if binary == "." {
		t.Skipf("%s is not set: the streamsim node test needs a built streams-simulator binary", streamsimBinaryVariable)
	}
	info := builtBinary(t, binary)
	repository := filepath.Dir(filepath.Dir(binary))
	if head := headCommitTime(t, repository); info.ModTime().Before(head) {
		t.Skipf("%s was built at %s, before the streamsim HEAD commit at %s: rebuild it", binary, info.ModTime(), head)
	}
	return binary, repository
}

func builtBinary(t *testing.T, binary string) os.FileInfo {
	t.Helper()
	bin, err := os.OpenRoot(filepath.Dir(binary))
	if err != nil {
		t.Skipf("%s=%s: %v", streamsimBinaryVariable, binary, err)
	}
	defer func() { _ = bin.Close() }()
	info, err := bin.Stat(filepath.Base(binary))
	if err != nil {
		t.Skipf("%s=%s: %v", streamsimBinaryVariable, binary, err)
	}
	return info
}

func headCommitTime(t *testing.T, repository string) time.Time {
	t.Helper()
	git := exec.CommandContext(t.Context(), "git", "log", "-1", "--format=%ct")
	git.Dir = repository
	output, err := git.Output()
	if err != nil {
		t.Skipf("%s is not in a git checkout: %v", repository, err)
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		t.Fatalf("HEAD commit time %q: %v", output, err)
	}
	return time.Unix(seconds, 0)
}

func assertEveryBarrierAnswered(t *testing.T, journal string) {
	t.Helper()
	counts := wireRecordCounts(t, journal)
	if counts["node NSYNC"] == 0 || counts["node NSYNC"] != counts["host HSYNC"] || counts["host STATE"] == 0 || counts["node NDEATH"] != 1 {
		t.Fatalf("%s: wire records %v, want every NSYNC answered by an HSYNC, a STATE and one NDEATH", journal, counts)
	}
}

func wireRecordCounts(t *testing.T, journal string) map[string]int {
	t.Helper()
	file, err := os.Open(journal)
	if err != nil {
		t.Fatalf("open the journal: %v", err)
	}
	defer func() { _ = file.Close() }()
	counts := map[string]int{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var entry struct {
			Type      string         `json:"type"`
			Direction string         `json:"direction"`
			Record    map[string]any `json:"record"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil && entry.Type == "wire" {
			counts[entry.Direction+" "+entry.Record["type"].(string)]++
		}
	}
	return counts
}

func copyFile(t *testing.T, from, toDir string) {
	t.Helper()
	raw, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(toDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(filepath.Base(from), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
