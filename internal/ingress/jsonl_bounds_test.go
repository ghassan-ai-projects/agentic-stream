package ingress

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestBoundedTraceLinesPreserveBoundaries(t *testing.T) {
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
			reader := bufio.NewReaderSize(strings.NewReader(tc.input), 16)
			line, oversized, err := readBoundedLine(reader, tc.limit)
			if err != nil || string(line) != tc.first || oversized != tc.oversized {
				t.Fatalf("first line=%q oversized=%v err=%v", line, oversized, err)
			}
			line, oversized, err = readBoundedLine(reader, tc.limit)
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
