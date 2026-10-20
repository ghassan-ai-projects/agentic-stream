package architecture

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// domainSpecPattern names the SituationSpecs whose declared words production
// Go code may not repeat: the domains the product ships. Test specs under
// internal/**/testdata are not read. They are test inputs, not domains, and
// any word a test chooses (a phase "ok", a preset "default") would turn the
// runtime's own vocabulary into a finding; today they add no word that the
// examples do not already declare.
const domainSpecPattern = "examples/*/*.situation.yaml"

// minDomainVocabulary is a floor under the vocabulary the shipped specs
// declare, so a moved or renamed example directory cannot turn the gate into
// a scan for nothing. Raise it when examples grow; lower it only with the
// examples it no longer counts.
const minDomainVocabulary = 35

type domainSpecWords struct {
	Inputs []struct {
		EventType  string `yaml:"eventType"`
		EntityType string `yaml:"entityType"`
	} `yaml:"inputs"`
	Situation struct {
		Type   string `yaml:"type"`
		Phases []struct {
			Name string `yaml:"name"`
		} `yaml:"phases"`
	} `yaml:"situation"`
	Actions struct {
		Intents []struct {
			Type            string `yaml:"type"`
			ParameterSchema struct {
				Properties map[string]struct {
					Enum []any `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"parameterSchema"`
			Presets map[string]any `yaml:"presets"`
		} `yaml:"intents"`
	} `yaml:"actions"`
}

func loadDomainVocabulary(t *testing.T, root string) []domainWord {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, domainSpecPattern))
	if err != nil || len(paths) == 0 {
		t.Fatalf("find shipped specs: %v (%d found)", err, len(paths))
	}
	words := make(map[domainWord]bool)
	for _, path := range paths {
		for _, word := range specWords(t, path) {
			words[word] = true
		}
	}
	return sortedVocabulary(t, words)
}

func sortedVocabulary(t *testing.T, words map[domainWord]bool) []domainWord {
	t.Helper()
	vocabulary := make([]domainWord, 0, len(words))
	for word := range words {
		vocabulary = append(vocabulary, word)
	}
	if len(vocabulary) < minDomainVocabulary {
		t.Fatalf("the shipped specs declare %d domain words, fewer than the floor %d; an example moved or the pattern %s no longer finds it", len(vocabulary), minDomainVocabulary, domainSpecPattern)
	}
	slices.SortFunc(vocabulary, func(a, b domainWord) int { return strings.Compare(a.word, b.word) })
	return vocabulary
}

func specWords(t *testing.T, path string) []domainWord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec domainSpecWords
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	words := []domainWord{{spec.Situation.Type, exactWord}}
	for _, input := range spec.Inputs {
		words = append(words, domainWord{input.EventType, containedWord}, domainWord{input.EntityType, exactWord})
	}
	for _, phase := range spec.Situation.Phases {
		words = append(words, domainWord{phase.Name, exactWord})
	}
	return append(words, intentWords(spec)...)
}

func intentWords(spec domainSpecWords) []domainWord {
	var words []domainWord
	for _, intent := range spec.Actions.Intents {
		words = append(words, domainWord{intent.Type, containedWord})
		for preset := range intent.Presets {
			words = append(words, domainWord{preset, exactWord})
		}
		for _, property := range intent.ParameterSchema.Properties {
			for _, value := range property.Enum {
				words = append(words, domainWord{fmt.Sprint(value), exactWord})
			}
		}
	}
	return words
}
