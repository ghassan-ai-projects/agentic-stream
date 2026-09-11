# A-077 · `internal/decisions/validator_test.go`

LOC: 436 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T4: unit tests for the decisions package depend only on fixtures owned by this package, not on documents under `docs/`.

## Findings
- **[MED] F1. Unit test coupled to a docs fixture outside the package** — `internal/decisions/validator_test.go:77`. `TestValidateCompensatingIntentTypes` compiles `../../docs/design/examples/predictive-maintenance.situation.yaml` to derive `specAllowed`. A unit test for decision validation now breaks with a confusing path error if the docs tree is reorganized, and the "allowed by spec" subtests silently depend on whichever intents the example happens to declare. Compile a minimal inline spec instead (the pattern already exists as `minimalSpecYAML()` in `internal/spec/compiler_test.go:235`) containing exactly the downgrade/withdraw intent types this test reasons about, or add a testdata YAML under `internal/decisions/testdata/`.
- **[LOW] F2. Helpers panic instead of failing the test** — `internal/decisions/validator_test.go:318,397,416`. `validInput()`, `validIntent()`, and `refreshIntentDigest()` call `panic(err)` on digest/marshal failures because they lack `*testing.T`. Acceptable but noisy; pass `t` (or return errors) so failures report as test failures with position, not stack traces.

## Checked, not an issue
- T1: assertions check typed `ValidationError.Reason` values (never error strings alone) and validated `Result` fields; failure messages name the expected reason. No tautologies.
- T2: deterministic — fixed `Now` (`time.Date(2026, 8, 12, …)`), fixed expiry timestamps, no network, no sleeps, no goroutines.
- T3: table-driven with `t.Run()` where natural (`TestValidateRejectsSecurityAndBindingFailures`, `TestValidateRejectsPresetSchemaAndEvidenceAttacks`); each subtest builds a fresh `validDecision()`, so mutations don't leak.
- T4: strong edge coverage — digest tamper, duplicate raw JSON key, implicit abstention, preset mismatch, ungrounded evidence, risk ceiling, expiry, catalog-missing fail-closed, `CompileIntentCatalog` failure modes.
- T5: `zeros`/`ones`/`testCatalog` are the only definitions in the package; no cross-file duplication.
