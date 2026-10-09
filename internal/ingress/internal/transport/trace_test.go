package transport

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundedReaderPreservesLineBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, input, first string
		limit              int
		oversized          bool
	}{
		{"at limit", "abc\nx\n", "abc\n", 4, false},
		{"over limit", "abcd\nx\n", "abcd", 4, true},
		{"multiple buffers", strings.Repeat("a", 80) + "\nx\n", "aaaa", 4, true},
		{"unterminated", "abc", "abc", 4, false},
		{"unterminated oversized", strings.Repeat("a", 80), "aaaa", 4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := NewBoundedReader(strings.NewReader(tc.input), tc.limit)
			line, oversized, err := reader.Next()
			if err != nil || string(line) != tc.first || oversized != tc.oversized {
				t.Fatalf("first line=%q oversized=%v err=%v", line, oversized, err)
			}
			line, oversized, err = reader.Next()
			if strings.HasSuffix(tc.input, "x\n") {
				if err != nil || string(line) != "x\n" || oversized {
					t.Fatalf("resynchronized line=%q oversized=%v err=%v", line, oversized, err)
				}
			} else if !errors.Is(err, io.EOF) {
				t.Fatalf("end of input err=%v", err)
			}
		})
	}
}

func TestEachScannedLineCountsBlankLinesAndStopsOnError(t *testing.T) {
	t.Parallel()
	var lines []string
	collect := func(line []byte) error { lines = append(lines, string(line)); return nil }
	if err := EachScannedLine(strings.NewReader("a\n\nb\n"), collect); err != nil || len(lines) != 3 || lines[1] != "" {
		t.Fatalf("lines = %q, err = %v, want a, a blank line, b", lines, err)
	}
	stop := errors.New("stop")
	if err := EachScannedLine(strings.NewReader("a\nb\n"), func([]byte) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("err = %v, want the callback error", err)
	}
}

func TestEachScannedLineRefusesALineBeyondTheScannerLimit(t *testing.T) {
	t.Parallel()
	err := EachScannedLine(strings.NewReader(strings.Repeat("x", 70*1024)), func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "read simulator trace") {
		t.Fatalf("err = %v, want a read failure for an over-long line", err)
	}
}

func TestOpenTraceNamesTheMissingFile(t *testing.T) {
	t.Parallel()
	_, err := OpenTrace(filepath.Join(t.TempDir(), "absent"), "trace file")
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "open trace file") {
		t.Fatalf("err = %v, want fs.ErrNotExist wrapped by open trace file", err)
	}
}
