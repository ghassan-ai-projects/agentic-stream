package campaign

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	errRecordingExhausted = errors.New("the recording has no further answer")
)

// Recording keeps every answer the worker gave, in order, so a campaign can
// be replayed from it without the worker deciding again. A replay serves each
// subject's answers in the order they were given.
type Recording struct {
	mu      sync.Mutex
	path    string
	replay  bool
	entries []recorded
}

type recorded struct {
	Kind    string          `json:"kind"`
	Subject string          `json:"subject"`
	Answer  json.RawMessage `json:"answer"`
}

// Record returns a recording that appends each new answer to path.
func Record(path string) *Recording { return &Recording{path: path} }

// Replay returns a recording that serves the answers kept in path in order.
func Replay(path string) (*Recording, error) {
	data, err := readIn(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("read the recording: %w", err)
	}
	entries, err := decodeRecorded(data)
	if err != nil {
		return nil, err
	}
	return &Recording{path: path, replay: true, entries: entries}, nil
}

func decodeRecorded(data []byte) ([]recorded, error) {
	var entries []recorded
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var entry recorded
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode recording line %d: %w", len(entries)+1, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan the recording: %w", err)
	}
	return entries, nil
}

func (r *Recording) next(kind, subject string, fresh func() (any, error), into any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := r.answer(kind, subject, fresh)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("decode the %s answer for %s: %w", kind, subject, err)
	}
	return nil
}

func (r *Recording) answer(kind, subject string, fresh func() (any, error)) (json.RawMessage, error) {
	if r.replay {
		return r.replayed(kind, subject)
	}
	given, err := fresh()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(given)
	if err != nil {
		return nil, fmt.Errorf("encode the %s answer for %s: %w", kind, subject, err)
	}
	return data, r.append(recorded{Kind: kind, Subject: subject, Answer: data})
}

func (r *Recording) replayed(kind, subject string) (json.RawMessage, error) {
	for i, entry := range r.entries {
		if entry.Kind == kind && entry.Subject == subject {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return entry.Answer, nil
		}
	}
	return nil, fmt.Errorf("%w: %s %s", errRecordingExhausted, kind, subject)
}

func (r *Recording) append(entry recorded) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode the recording line: %w", err)
	}
	file, err := r.open()
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(line, '\n'))
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("append to the recording: %w", err)
	}
	return nil
}

func (r *Recording) open() (*os.File, error) {
	root, err := os.OpenRoot(filepath.Dir(r.path))
	if err != nil {
		return nil, fmt.Errorf("open the recording folder: %w", err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(filepath.Base(r.path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the recording: %w", err)
	}
	return file, nil
}
