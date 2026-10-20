package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// builtinDomainWord is a word a spec may declare that Agentic Stream itself
// defines, with the only production files that may spell it and why.
type builtinDomainWord struct {
	files  []string
	reason string
}

// builtinDomainWords is the reviewed allow-list. A new entry is a reviewed
// claim that the word is the runtime's own, not a domain's.
var builtinDomainWords = map[string]builtinDomainWord{
	"install_watch_condition": {
		files: []string{
			"internal/watch/internal/domain/condition.go",
			"internal/decisions/internal/domain/binding.go",
		},
		reason: "the watch install route is Agentic Stream's own effect route (watch.InstallRoute); specs declare it to allow it, the watch module owns it",
	},
	"resolved": {
		files: []string{
			"internal/situations/internal/domain/evaluate.go",
			"internal/policy/internal/domain/routing.go",
			"internal/policy/internal/app/approval.go",
		},
		reason: "the terminal Situation phase the lifecycle defines (PhaseResolved); policy also uses the word for an approval that is no longer pending, unrelated to any domain",
	},
	"default": {
		files: []string{
			"internal/decisions/internal/domain/intent_parameters.go",
			"internal/contractsv1/internal/domain/declarations.go",
		},
		reason: "the preset name the decision validator compares a decision's parameters against (presets.default), and the default tenant id; both are runtime conventions",
	},
}

// domainKnowledgeExemptPaths are production paths that emulate a domain by
// design and never run in a deployment.
var domainKnowledgeExemptPaths = map[string]string{
	"internal/executor/fixture/":                "the deterministic demo executor proposes a fixed example decision; DemoMode admits it, a production pipeline refuses it",
	"internal/testsupport/executorconformance/": "the executor conformance suite drives executors with a fixed example decision; tests import it, production code does not",
}

type wordMatch int

const (
	exactWord wordMatch = iota
	containedWord
)

type domainWord struct {
	word  string
	match wordMatch
}

type domainWordUse struct {
	word string
	node ast.Node
}

// TestProductionCodeHoldsNoDomainWords keeps domain knowledge in data: no
// production string literal names an intent type, event type, entity type,
// situation type, phase, preset or parameter enum value that a shipped
// SituationSpec declares, except the runtime's own built-ins.
func TestProductionCodeHoldsNoDomainWords(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	vocabulary := loadDomainVocabulary(t, repo.root)
	used := make(map[string]bool)
	for _, file := range repo.production {
		if file.generated || domainKnowledgeExempt(file.rel) {
			continue
		}
		for _, use := range domainWordUses(file.syntax, vocabulary) {
			if builtinAllows(use.word, file.rel) {
				used[use.word+" "+file.rel] = true
				continue
			}
			t.Errorf("%s: literal names the spec word %q; read it from the spec or the device catalog, or list it in builtinDomainWords with a reason", repo.position(use.node), use.word)
		}
	}
	reportStaleBuiltins(t, used)
	reportStaleExemptions(t, repo)
}

// TestTheDomainWordGateCatchesASpecWordInCode proves the gate is not vacuous:
// a literal that names a shipped intent type is found, an exact-match word is
// not found inside a longer literal, and an event type is found inside one.
func TestTheDomainWordGateCatchesASpecWordInCode(t *testing.T) {
	t.Parallel()

	vocabulary := loadDomainVocabulary(t, loadRepository(t).root)
	source := `package synthetic
var route = "set_indicator"
var column = "over_ceiling_count"
var ref = "zone.temp.observed/1.0"
`
	syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, use := range domainWordUses(syntax, vocabulary) {
		found = append(found, use.word)
	}
	if !slices.Equal(found, []string{"set_indicator", "zone.temp.observed"}) {
		t.Fatalf("synthetic literals found %v, want [set_indicator zone.temp.observed]", found)
	}
}

func domainKnowledgeExempt(rel string) bool {
	for prefix := range domainKnowledgeExemptPaths {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func builtinAllows(word, rel string) bool {
	builtin, listed := builtinDomainWords[word]
	return listed && slices.Contains(builtin.files, rel)
}

func reportStaleBuiltins(t *testing.T, used map[string]bool) {
	t.Helper()
	for word, builtin := range builtinDomainWords {
		for _, file := range builtin.files {
			if !used[word+" "+file] {
				t.Errorf("builtinDomainWords allows %q in %s, which no longer spells it or no shipped spec declares it; remove the stale entry", word, file)
			}
		}
	}
}

func reportStaleExemptions(t *testing.T, repo *repository) {
	t.Helper()
	for prefix := range domainKnowledgeExemptPaths {
		if len(repo.filesUnder(strings.TrimSuffix(prefix, "/"))) == 0 {
			t.Errorf("domainKnowledgeExemptPaths names %s, which has no production files; remove the stale entry", prefix)
		}
	}
}

func domainWordUses(syntax *ast.File, vocabulary []domainWord) []domainWordUse {
	var uses []domainWordUse
	ast.Inspect(syntax, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		for _, word := range vocabulary {
			if word.match == exactWord && value == word.word || word.match == containedWord && strings.Contains(value, word.word) {
				uses = append(uses, domainWordUse{word: word.word, node: literal})
			}
		}
		return true
	})
	return uses
}

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
	vocabulary := make([]domainWord, 0, len(words))
	for word := range words {
		vocabulary = append(vocabulary, word)
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
