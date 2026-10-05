package transport

import (
	"bufio"
	"fmt"
	"os"
)

// TraceLines streams every line of a trace file to the visitor. Opening and
// scanning failures are wrapped; line interpretation stays with the caller.
func TraceLines(path string, each func(line []byte)) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open trace: %w", err)
	}
	defer func() { _ = file.Close() }()
	return scanTraceLines(bufio.NewScanner(file), each)
}

func scanTraceLines(scanner *bufio.Scanner, each func(line []byte)) error {
	for scanner.Scan() {
		each(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan trace: %w", err)
	}
	return nil
}

// WithRunDirectory creates a private directory for one repeated-replay proof
// and removes it when the work finishes.
func WithRunDirectory(work func(dir string) error) error {
	dir, err := os.MkdirTemp("", "agentic-stream-replay-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	return work(dir)
}
