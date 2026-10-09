package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// maxProductionFileLines is quality-bar rule Q3: production files stay under
// 300 lines. See .agents/context/quality-bar.md.
const maxProductionFileLines = 299

// TestProductionFileSize enforces quality-bar rule Q3.
func TestProductionFileSize(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).production {
		if !file.generated && file.lines > maxProductionFileLines {
			t.Errorf("%s has %d lines; split it by responsibility to under %d", file.rel, file.lines, maxProductionFileLines+1)
		}
	}
}

// TestProductionFunctionLengthBar prevents the lint gate from drifting away
// from the clean-function bar recorded in AGENTS.md.
func TestProductionFunctionLengthBar(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(loadRepository(t).root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Linters struct {
			Enable   []string `yaml:"enable"`
			Settings struct {
				Funlen struct {
					Lines          int  `yaml:"lines"`
					Statements     int  `yaml:"statements"`
					IgnoreComments bool `yaml:"ignore-comments"`
				} `yaml:"funlen"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(config.Linters.Enable, "funlen") {
		t.Fatal("production function length lint must be enabled")
	}
	limit := config.Linters.Settings.Funlen
	if limit.Lines != 15 || limit.Statements != 15 || !limit.IgnoreComments {
		t.Fatalf("production function bar = %+v; require 15 body lines, 15 statements, excluding comments", limit)
	}
}

// TestGeneratedProtocolImportsStayGeneratorOwned keeps formatter hooks from
// conflicting with the byte-for-byte protocol generation gate.
func TestGeneratedProtocolImportsStayGeneratorOwned(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(loadRepository(t).root, ".pre-commit-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Repos []struct {
			Hooks []struct {
				ID      string `yaml:"id"`
				Exclude string `yaml:"exclude"`
			} `yaml:"hooks"`
		} `yaml:"repos"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	for _, repo := range config.Repos {
		for _, hook := range repo.Hooks {
			if hook.ID != "go-imports" {
				continue
			}
			excluded, err := regexp.MatchString(hook.Exclude, "proto/agenticstream/runtime/v1/runtime-v1.pb.go")
			if err != nil || !excluded {
				t.Fatalf("generated imports exclusion = %q, error = %v", hook.Exclude, err)
			}
			excluded, err = regexp.MatchString(hook.Exclude, "internal/policy/policy.go")
			if err != nil || excluded {
				t.Fatalf("production imports must still be checked: exclusion=%q error=%v", hook.Exclude, err)
			}
			return
		}
	}
	t.Fatal("go-imports hook missing")
}
