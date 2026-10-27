package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var (
	ErrPlaybookChanged = errors.New("the playbook file changed since it was loaded")
	ErrNoPlaybookFile  = errors.New("no playbook file to write")
)

type playbookFile struct {
	mu       sync.Mutex
	path     string
	spec     *spec.CompiledSpec
	expected string
}

func (f *playbookFile) update(change func(*spec.CompiledPlaybook) bool) (string, error) {
	if f == nil || f.path == "" {
		return "", ErrNoPlaybookFile
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	current, err := f.current()
	if err != nil || !change(current) {
		return f.expected, err
	}
	written, err := f.write(current)
	if err != nil {
		return "", err
	}
	f.expected = written
	return written, nil
}

func (f *playbookFile) current() (*spec.CompiledPlaybook, error) {
	current, err := spec.CompilePlaybookFile(f.spec, f.path)
	if err != nil {
		return nil, fmt.Errorf("read the playbook file: %w", err)
	}
	if current.Digest != f.expected {
		return nil, fmt.Errorf("%w: %s is %s, %s was loaded", ErrPlaybookChanged, f.path, current.Digest, f.expected)
	}
	return current, nil
}

func (f *playbookFile) write(book *spec.CompiledPlaybook) (string, error) {
	document, compiled, err := compileCandidate(f.spec, book)
	if err != nil {
		return "", err
	}
	return compiled.Digest, f.swapIn(document)
}

func compileCandidate(compiled *spec.CompiledSpec, book *spec.CompiledPlaybook) ([]byte, *spec.CompiledPlaybook, error) {
	document, err := spec.MarshalPlaybook(book)
	if err != nil {
		return nil, nil, fmt.Errorf("write the candidate playbook: %w", err)
	}
	checked, err := spec.CompilePlaybook(compiled, document)
	if err != nil {
		return nil, nil, fmt.Errorf("check the candidate playbook: %w", err)
	}
	return document, checked, nil
}

func (f *playbookFile) swapIn(document []byte) error {
	temporary := filepath.Join(filepath.Dir(f.path), "."+filepath.Base(f.path)+".next")
	if err := os.WriteFile(temporary, document, 0o600); err != nil {
		return fmt.Errorf("write the new playbook: %w", err)
	}
	if err := os.Rename(temporary, f.path); err != nil {
		return fmt.Errorf("swap in the new playbook file: %w", err)
	}
	return nil
}
