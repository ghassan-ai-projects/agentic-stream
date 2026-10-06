package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

// OpenTrace opens a trace file for reading; what names it in the error.
func OpenTrace(path, what string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", what, err)
	}
	return f, nil
}

// EachScannedLine hands every line of r to fn, scanner-bounded, counting blank
// lines; fn decides what a blank line means. A line longer than the scanner
// limit fails the read.
func EachScannedLine(r io.Reader, fn func(line []byte) error) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if err := fn(scanner.Bytes()); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read simulator trace: %w", err)
	}
	return nil
}

// BoundedReader reads newline-terminated lines of at most max bytes.
type BoundedReader struct {
	reader *bufio.Reader
	max    int
}

// NewBoundedReader caps per-line memory at max bytes.
func NewBoundedReader(r io.Reader, max int) *BoundedReader {
	return &BoundedReader{reader: bufio.NewReaderSize(r, max), max: max}
}

// Next reads one line. When a line exceeds the bound it returns the truncated
// prefix with tooLarge=true and resynchronizes to the start of the next line
// (discarding the overlong remainder) so ingestion continues rather than
// aborting. It returns io.EOF only when no bytes remained to read. Unlike the
// live-socket reader, which drops its connection after an oversized frame, this
// resyncs deterministically for a file trace.
func (b *BoundedReader) Next() ([]byte, bool, error) {
	var line []byte
	over := false
	for {
		part, err := b.reader.ReadSlice('\n')
		line, over = appendBounded(line, part, b.max, over)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return finishBoundedLine(line, over, err)
		}
		if over {
			return line, true, skipOverlongRemainder(b.reader)
		}
	}
}

// finishBoundedLine completes a line at a newline or at end of input; io.EOF
// is returned only when no bytes remained.
func finishBoundedLine(line []byte, over bool, err error) ([]byte, bool, error) {
	switch {
	case err == nil:
		return line, over, nil
	case errors.Is(err, io.EOF) && (len(line) > 0 || over):
		return line, over, nil
	case errors.Is(err, io.EOF):
		return nil, false, io.EOF
	default:
		return line, over, fmt.Errorf("read line: %w", err)
	}
}

// appendBounded appends part to line while line stays within max bytes, and
// reports whether the line has overflowed.
func appendBounded(line, part []byte, max int, over bool) ([]byte, bool) {
	if len(line)+len(part) <= max {
		return append(line, part...), over
	}
	if room := max - len(line); room > 0 {
		line = append(line, part[:room]...)
	}
	return line, true
}

// skipOverlongRemainder discards the rest of an oversized line; reaching the
// end of input while doing so is not an error.
func skipOverlongRemainder(r *bufio.Reader) error {
	if err := discardToNewline(r); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// discardToNewline consumes the reader up to and including the next newline,
// used to resynchronize after an oversized line whose prefix was quarantined.
func discardToNewline(r *bufio.Reader) error {
	for {
		_, err := r.ReadSlice('\n')
		if err == nil {
			return nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return fmt.Errorf("discard to newline: %w", err)
	}
}
