package domain_test

import (
	"errors"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

const promotionIntents = `
    - type: service
      risk: R1
      parameterSchema:
        type: object
        properties:
          entity_id:
            type: string
          order:
            type: string
        additionalProperties: false
      modelWritableFields: [order]
      presets:
        wear:
          order: wear
        drift:
          order: drift
      policy: automatic
    - type: halt
      risk: R3
      parameterSchema:
        type: object
        properties:
          entity_id:
            type: string
        additionalProperties: false
      presets:
        default: {}
    - type: trip
      risk: R1
      emergency: true
      parameterSchema:
        type: object
        properties:
          entity_id:
            type: string
        additionalProperties: false
      presets:
        default: {}
    - type: interlock
      risk: R1
      safetyFunction: true
      parameterSchema:
        type: object
        properties:
          entity_id:
            type: string
        additionalProperties: false
      presets:
        default: {}
promotion:
  settleWindows:
    wear: 24h
  contextFeatures: [count_value]
  remedyMap:
    drift: [halt]
  recovery:
    wear: {reading: mean_value, baseline: count_value, tolerance: 0.1}
  envelopeMargin: 0.1
`

func promotionSpecYAML(edits ...string) string {
	source := editedSpec(
		"      costMicrounits: 0\n",
		"      costMicrounits: 0\n    diagnosisCatalog: '[{\"code\":\"unknown\"},{\"code\":\"wear\"},{\"code\":\"drift\"},{\"code\":\"noise\"}]'\n",
		"    output: mean_value\n",
		"    output: mean_value\n  - name: op2\n    kind: aggregate\n    inputs: [temp]\n    field: data.value\n    window: w1\n    aggregate: count\n    output: count_value\n",
	)
	source += promotionIntents
	for i := 0; i+1 < len(edits); i += 2 {
		source = strings.Replace(source, edits[i], edits[i+1], 1)
	}
	return source
}

const playbookYAML = `apiVersion: agentic-stream/v1
kind: Playbook
metadata:
  name: site
  spec: test
rules:
  - id: wear-rule
    version: 1
    diagnosisCode: wear
    when: features.mean_value > 4.0
    recommend:
      intent: service
      preset: wear
    envelope:
      mean_value: {min: 4.5, max: 6.0}
    cap:
      mean_value: {min: 4.0, max: 8.0}
      count_value: {min: 1, max: 100}
    pinnedKind:
      name: pump
      digest: sha256:17f5e82f53bc294d167f8543598ba2d49f5c60373985b78aa9ebe17288e79f19
    mode: shadow
    rateCapPerHour: 4
    provenance:
      sourceEpisodes: [ep-1, ep-2, ep-3]
`

func compilePromotionSpec(t *testing.T, edits ...string) *domain.CompiledSpec {
	t.Helper()
	compiled, err := compileSource(t, promotionSpecYAML(edits...))
	if err != nil {
		t.Fatalf("compile promotion spec: %v", err)
	}
	return compiled
}

func TestCompilePlaybookSealsItsOwnDigestAndDefaults(t *testing.T) {
	t.Parallel()
	compiled := compilePromotionSpec(t)
	specDigest := compiled.Digest
	playbook, err := domain.CompilePlaybook(compiled, []byte(playbookYAML))
	if err != nil {
		t.Fatalf("compile playbook: %v", err)
	}
	if compiled.Digest != specDigest || playbook.Digest == "" || playbook.Digest == compiled.Digest {
		t.Fatalf("spec digest %s, playbook digest %s: the playbook must have its own digest", compiled.Digest, playbook.Digest)
	}
	rule := playbook.Rules[0]
	if rule.AuditEvery != 3 || rule.ShadowExit.Agreeing != 10 || rule.ShadowExit.Disagreeing != 0 {
		t.Fatalf("defaults = audit every %d, shadow exit %+v; want 3 and 10/0", rule.AuditEvery, *rule.ShadowExit)
	}
	if strings.Join(rule.Reads, ",") != "mean_value" {
		t.Fatalf("reads = %v, want [mean_value]", rule.Reads)
	}
	edited, err := domain.CompilePlaybook(compiled, []byte(strings.Replace(playbookYAML, "rateCapPerHour: 4", "rateCapPerHour: 5", 1)))
	if err != nil || edited.Digest == playbook.Digest {
		t.Fatalf("an edited rule must change the playbook digest: %v", err)
	}
}

func TestCompilePlaybookRefusesEveryUnsafeOrUnknownRule(t *testing.T) {
	t.Parallel()
	compiled := compilePromotionSpec(t)
	cases := []struct{ name, from, to, path string }{
		{"another spec", "spec: test", "spec: other", "metadata.spec"},
		{"unknown diagnosis", "diagnosisCode: wear", "diagnosisCode: unknown", "rules[0].diagnosisCode"},
		{"uncataloged diagnosis", "diagnosisCode: wear", "diagnosisCode: rust", "rules[0].diagnosisCode"},
		{"no settle window", "diagnosisCode: wear", "diagnosisCode: noise", "rules[0].diagnosisCode"},
		{"undeclared intent", "intent: service", "intent: refuel", "rules[0].recommend.intent"},
		{"undeclared preset", "preset: wear", "preset: rust", "rules[0].recommend.preset"},
		{"risk R3", "intent: service\n      preset: wear", "intent: halt\n      preset: default", "rules[0].recommend.intent"},
		{"emergency", "intent: service\n      preset: wear", "intent: trip\n      preset: default", "rules[0].recommend.intent"},
		{"safety function", "intent: service\n      preset: wear", "intent: interlock\n      preset: default", "rules[0].recommend.intent"},
		{"remedy map", "diagnosisCode: wear\n    when", "diagnosisCode: drift\n    when", "rules[0].recommend.intent"},
		{"unknown feature", "when: features.mean_value > 4.0", "when: features.vibration > 4.0", "rules[0].when"},
		{"situation read", "when: features.mean_value > 4.0", "when: situation.phase == 'alert'", "rules[0].when"},
		{"dynamic read", "when: features.mean_value > 4.0", "when: features['mean_value'] > 4.0", "rules[0].when"},
		{"not boolean", "when: features.mean_value > 4.0", "when: features.mean_value", "rules[0].when"},
		{"cap on an unread feature", "count_value: {min: 1, max: 100}", "other_value: {min: 1, max: 100}", "rules[0].cap.other_value"},
		{"envelope outside cap", "mean_value: {min: 4.5, max: 6.0}", "mean_value: {min: 4.5, max: 9.0}", "rules[0].envelope.mean_value"},
		{"envelope without cap", "mean_value: {min: 4.5, max: 6.0}\n", "mean_value: {min: 4.5, max: 6.0}\n      count_value: {min: 2, max: 3}\n      extra: {min: 1, max: 2}\n", "rules[0].envelope.extra"},
		{"inverted cap", "mean_value: {min: 4.0, max: 8.0}", "mean_value: {min: 8.0, max: 4.0}", "rules[0].cap.mean_value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := strings.Replace(playbookYAML, tc.from, tc.to, 1)
			if source == playbookYAML {
				t.Fatalf("edit %q not applied", tc.from)
			}
			assertCompileErrorAt(t, compilePlaybookError(t, compiled, source), tc.path)
		})
	}
}

func TestCompilePlaybookRefusesADuplicateRuleAndAMissingCatalog(t *testing.T) {
	t.Parallel()
	compiled := compilePromotionSpec(t)
	twice := playbookYAML + strings.SplitN(playbookYAML, "rules:\n", 2)[1]
	assertCompileErrorAt(t, compilePlaybookError(t, compiled, twice), "rules[1].id")
	uncataloged, err := compileSource(t, editedSpec("name: test\n", "name: test\n"))
	if err != nil {
		t.Fatal(err)
	}
	assertCompileErrorAt(t, compilePlaybookError(t, uncataloged, playbookYAML), "rules")
}

func TestCompilePlaybookRejectsAnUnknownField(t *testing.T) {
	t.Parallel()
	compiled := compilePromotionSpec(t)
	source := strings.Replace(playbookYAML, "    mode: shadow\n", "    mode: shadow\n    reads: [mean_value]\n", 1)
	if _, err := domain.CompilePlaybook(compiled, []byte(source)); err == nil || !strings.Contains(err.Error(), "reads") {
		t.Fatalf("compile with a reads field = %v, want it refused", err)
	}
}

func TestCompileChecksThePromotionBlock(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, from, to, path string }{
		{"settle window code", "    wear: 24h", "    rust: 24h", "promotion.settleWindows.rust"},
		{"context feature", "contextFeatures: [count_value]", "contextFeatures: [load]", "promotion.contextFeatures[0]"},
		{"remedy code", "    drift: [halt]", "    rust: [halt]", "promotion.remedyMap.rust"},
		{"remedy intent", "    drift: [halt]", "    drift: [refuel]", "promotion.remedyMap.drift"},
		{"recovery code", "    wear: {reading", "    rust: {reading", "promotion.recovery.rust"},
		{"recovery without a settle window", "    wear: {reading", "    drift: {reading", "promotion.recovery.drift"},
		{"recovery reading", "reading: mean_value", "reading: load", "promotion.recovery.wear.reading"},
		{"recovery baseline", "baseline: count_value", "baseline: load", "promotion.recovery.wear.baseline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileSource(t, promotionSpecYAML(tc.from, tc.to))
			assertCompileErrorAt(t, err, tc.path)
		})
	}
}

func TestCompileRefusesARecoveryCheckThatIsNotWellFormed(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, from, to, want string }{
		{"zero tolerance", "tolerance: 0.1", "tolerance: 0", "recovery"},
		{"no tolerance", ", tolerance: 0.1}", "}", "recovery"},
		{"unknown field", "tolerance: 0.1}", "tolerance: 0.1, window: 5m}", "window"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := compileSource(t, promotionSpecYAML(tc.from, tc.to)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("compile = %v, want the recovery check refused", err)
			}
		})
	}
}

func TestPromotionAdditionsLeaveAnExistingSpecDigestUnchanged(t *testing.T) {
	t.Parallel()
	plain, err := compileSource(t, minimalSpecYAML())
	if err != nil {
		t.Fatal(err)
	}
	flagged, err := compileSource(t, editedSpec("      risk: R0\n", "      risk: R0\n      emergency: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Digest != flagged.Digest {
		t.Fatalf("an unset flag changed the digest: %s != %s", plain.Digest, flagged.Digest)
	}
	marked, err := compileSource(t, editedSpec("      risk: R0\n", "      risk: R0\n      emergency: true\n"))
	if err != nil || marked.Digest == plain.Digest {
		t.Fatalf("a set flag must change the digest: %v", err)
	}
}

func compilePlaybookError(t *testing.T, compiled *domain.CompiledSpec, source string) error {
	t.Helper()
	_, err := domain.CompilePlaybook(compiled, []byte(source))
	return err
}

func assertCompileErrorAt(t *testing.T, err error, path string) {
	t.Helper()
	var compileErr *domain.CompileError
	if !errors.As(err, &compileErr) || compileErr.Path != path {
		t.Fatalf("error = %v, want a compile error at %s", err, path)
	}
}

func TestAMarshaledPlaybookCompilesToTheSameDigest(t *testing.T) {
	t.Parallel()
	compiled := compilePromotionSpec(t)
	playbook, err := domain.CompilePlaybook(compiled, []byte(playbookYAML))
	if err != nil {
		t.Fatal(err)
	}
	written, err := domain.MarshalPlaybook(playbook)
	if err != nil {
		t.Fatal(err)
	}
	again, err := domain.CompilePlaybook(compiled, written)
	if err != nil || again.Digest != playbook.Digest {
		t.Fatalf("recompiled digest = %v (%v), want %s\n%s", again, err, playbook.Digest, written)
	}
}
