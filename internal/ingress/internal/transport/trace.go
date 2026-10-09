package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

func OpenTrace(path, what string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", what, err)
	}
	return f, nil
}

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

type BoundedReader struct {
	reader   *bufio.Reader
	max      int
	consumed int64
}

func NewBoundedReader(r io.Reader, max int) *BoundedReader {
	return &BoundedReader{reader: bufio.NewReaderSize(r, max), max: max}
}

func (b *BoundedReader) Next() ([]byte, bool, error) {
	var line []byte
	over := false
	for {
		part, err := b.reader.ReadSlice('\n')
		b.consumed += int64(len(part))
		line, over = appendBounded(line, part, b.max, over)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return finishBoundedLine(line, over, err)
		}
		if over {
			skipped, err := skipOverlongRemainder(b.reader)
			b.consumed += skipped
			return line, true, err
		}
	}
}

func (b *BoundedReader) Consumed() int64 { return b.consumed }

func OpenTraceFrom(path string, offset int64) (io.ReadCloser, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("open trace file: %w", err)
	}
	resumed, err := seekWithinFile(f, offset)
	if err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return f, resumed, nil
}

func seekWithinFile(f *os.File, offset int64) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat trace file: %w", err)
	}
	if offset <= 0 || info.Size() < offset {
		return false, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return false, fmt.Errorf("seek trace file: %w", err)
	}
	return true, nil
}

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

func appendBounded(line, part []byte, max int, over bool) ([]byte, bool) {
	if len(line)+len(part) <= max {
		return append(line, part...), over
	}
	if room := max - len(line); room > 0 {
		line = append(line, part[:room]...)
	}
	return line, true
}

func skipOverlongRemainder(r *bufio.Reader) (int64, error) {
	skipped, err := discardToNewline(r)
	if err != nil && !errors.Is(err, io.EOF) {
		return skipped, err
	}
	return skipped, nil
}

func discardToNewline(r *bufio.Reader) (int64, error) {
	var skipped int64
	for {
		part, err := r.ReadSlice('\n')
		skipped += int64(len(part))
		if err == nil {
			return skipped, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return skipped, fmt.Errorf("discard to newline: %w", err)
	}
}
