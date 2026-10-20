package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
)

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
			"internal/actionport/internal/domain/effect.go",
			"internal/decisions/internal/domain/binding.go",
		},
		reason: "the watch install route is Agentic Stream's own effect route (actionport.WatchInstallRoute, re-exported as watch.InstallRoute); the effect port reserves it so no device catalog may name it; specs declare it to allow it",
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

// generatedCodePaths are the only places a "Code generated ... DO NOT EDIT."
// file is skipped. A file elsewhere that carries the marker is scanned like
// hand-written code, so the marker cannot opt a file out of the gate.
var generatedCodePaths = map[string]string{
	"proto/agenticstream/runtime/v1/": "protoc-gen-go and protoc-gen-go-grpc output of the worker protocol; the generator spells only protocol names",
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
		if generatedCodeExempt(file) || hasPathPrefix(domainKnowledgeExemptPaths, file.rel) {
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
	reportStaleExemptions(t, repo, domainKnowledgeExemptPaths)
	reportStaleExemptions(t, repo, generatedCodePaths)
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

// TestOnlyKnownGeneratedPathsSkipTheDomainWordGate proves the generated
// marker opts a file out only under generatedCodePaths.
func TestOnlyKnownGeneratedPathsSkipTheDomainWordGate(t *testing.T) {
	t.Parallel()

	files := []struct {
		file goFile
		want bool
	}{
		{goFile{rel: "proto/agenticstream/runtime/v1/runtime-v1.pb.go", generated: true}, true},
		{goFile{rel: "internal/runtime/internal/app/routes.go", generated: true}, false},
		{goFile{rel: "proto/agenticstream/runtime/v1/handwritten.go"}, false},
	}
	for _, tt := range files {
		if got := generatedCodeExempt(tt.file); got != tt.want {
			t.Errorf("generatedCodeExempt(%s, generated=%t) = %t, want %t", tt.file.rel, tt.file.generated, got, tt.want)
		}
	}
}

func generatedCodeExempt(file goFile) bool {
	return file.generated && hasPathPrefix(generatedCodePaths, file.rel)
}

func hasPathPrefix(prefixes map[string]string, rel string) bool {
	for prefix := range prefixes {
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

func reportStaleExemptions(t *testing.T, repo *repository, prefixes map[string]string) {
	t.Helper()
	for prefix := range prefixes {
		if len(repo.filesUnder(strings.TrimSuffix(prefix, "/"))) == 0 {
			t.Errorf("exempt path %s has no production files; remove the stale entry", prefix)
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
