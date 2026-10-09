# canonicaljson

Status: done
Round: 2

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/canonicaljson` | 85.7% | 100% | 1.1 s | 1.1 s | 3 / 3 | 4 / 6 |
| `internal/canonicaljson/internal/domain` | 81.0% | 94.9% | 1.1 s | 1.2 s | 29 / 97 | 36 / 296 |

## Layout (domain tests, one file per subject)

| File | Proves |
| --- | --- |
| `encode_test.go` | `Marshal`: key order by UTF-16 code units, every JSON shape, typed values via the fallback, refusals |
| `text_test.go` | string escaping (RFC 8785 §3.2.2.2), `compareUTF16`, `utf16Units`, malformed Unicode |
| `number_test.go` | RFC 8785 Appendix B doubles, exponent placement, Go integer range, raw number spellings, malformed numbers, `scaleDigits` |
| `validate_test.go` | strict raw-JSON profile: strings, escapes, surrogates, duplicate keys, trailing values, structure |
| `digest_test.go` | the digest domains (frozen literals), `Digest`/`Seal`/`Verify`/`VerifySum` |
| `vectors_test.go` | the shared cross-repository corpus (accept, reject, native-only) |
| `stored_test.go`, `schema_test.go` | stored-document verification; offline schema compiler (unchanged apart from review) |
| facade: `canonicaljson_test.go`, `ruby_digest_parity_test.go` | delegation and the frozen Ruby digests |

## Findings and changes

### Removed
- `TestMarshalSortsObjectKeys`, `TestMarshalNested`, `TestMarshalFloat`, `TestMarshalRejectsNonFinite`, `TestMarshalRejectsNegativeZero`: subsumed by `encode_test.go` and `number_test.go` tables (T2).
- `TestDigestStable`, `TestDigestHasDomainSeparation`, `TestDecodeDigestRequiresCanonicalPrefix`, `TestDecodeDigestRejectsMalformedReferences`: folded into `TestDigestIgnoresKeyOrderAndIsBoundToItsDomain`, `TestVerifyBindsDigestToValueAndDomain` and a one-case wrapper test; the digest text rules are proven in `internal/kernel` (T2).
- `TestNativeIntegerOutsideExactRangeIsRejected`, `TestRawUnsafeIntegerSpellingsAreRejected`, `TestRawValidEscapedSurrogatePairIsAccepted`, `TestRawEscapedEquivalentDuplicateKeysAreRejected`, `TestExactlyRepresentableDoubleAtSafeBoundaryIsAccepted`, `TestNativeInvalidUnicodeIsRejected`: scattered single cases, now rows of the number, text and validate tables (T11).
- The facade comments (T11) and the Ruby parity header comment: names carry the meaning.

### Renamed or moved
- `ruby_parity_test.go` → `ruby_digest_parity_test.go` (`git mv`); `TestRubyDigestParity` → `TestDigestsMatchTheFrozenRubyVectors` (table).
- `TestScaleDigits` (`validate_test.go`) → `number_test.go` (it tests `number.go`).
- `TestSharedCanonicalizationVectors`, `TestNativeOnlyDoubleVectors`, `TestSharedRejectVectors` → `TestSharedAcceptVectorsCanonicalizeAndDigestAsRecorded`, `TestSharedNativeOnlyVectorsCanonicalizeGoDoubles`, `TestSharedRejectVectorsAreRefused`.

### Improved
- T6: the tests in `digest_test.go` and `vectors_test.go` (and their vector subtests) and the Ruby parity test were not parallel; all are. `runtime.Caller` path replaced by a relative path (tests run in the package directory).
- T4: the old tests asserted `err == nil`/`!= nil`; refusals now assert the rule (message). The native-only vectors were never read from the corpus file (the test hardcoded two cases); it now iterates `native_only` and fails if the corpus and the test disagree. The reject-vector Go values are matched by name and an unknown vector fails.
- Digest domains: `TestDomainsAreTheFrozenDigestNamespaces` pins all 17 literals (digest contract), their shape and distinctness.

### Added (RFC 8785 coverage, one table each)
- Key order: the RFC §3.2.3 sample, an astral key before U+E000 (the code-unit versus code-point difference), prefixes, case, digits.
- Number formatting: 22 Appendix B doubles by bit pattern; exponent boundaries at 1e-6/1e-7 and 1e20/1e21 (also through `float32`); NaN, ±Inf, -0 (native and `float32`) refused; every Go integer type at ±(2^53-1) and the refusals above; raw spellings (`1.0`, `1e2`, `100e-2`, `4.9e-324`, …) and the unsafe ones (`2^53`, `9007199254740993e0`, `90071992547409930e-1`, `1e21`, `1e400`, `-0`, `-0.0`, `-0e0`); malformed JSON numbers (`01`, `+1`, `.5`, `1.`, `0x10`, `NaN`, …).
- Escaping: short escapes, `\u00xx` lower case for the other controls, `/` and DEL verbatim, non-ASCII, U+2028/2029, astral and U+FFFF verbatim, five malformed-UTF-8 forms refused.
- Raw JSON: duplicate keys (nested, in arrays, escaped-equivalent), multiple values, truncated and malformed structure, lone surrogates in values and keys, upper case hex escapes.
- Typed values through the fallback path (`encodeMarshaled`, was 0%): structs with tags, pointers, typed slices and maps; unsupported kinds and NaN-in-struct refused.
- `TestFacadeCompilesSchemasOfflineWithFormatAssertions`: the facade `CompileSchema`/`CompileSchemaJSON` had no test; format assertions are on and an external `$ref` is refused.

### Speed
- Nothing slow (the whole domain package, 296 passing tests, runs in about 0.2 s without `-race`).

## Production code touched
- none

## Invariants proven here
- Foundation for digests in invariants 3 and 8: `TestSharedAcceptVectorsCanonicalizeAndDigestAsRecorded`, `TestDigestsMatchTheFrozenRubyVectors`, `TestDomainsAreTheFrozenDigestNamespaces`.

## Open items
- `containsSurrogate` (`text.go`) has an unreachable `true` branch: it ranges over a Go string after `utf8.ValidString`, and range never yields a surrogate. Candidate for removal (production change, left alone).
- Asymmetry, not changed: a native `float64` above 2^53 (for example `1e20`, `295147905179352830000`) is accepted and formatted positionally, while the same value spelled in raw JSON is refused as an unsafe integer. Pinned by the Appendix B rows; a design decision is needed before either side changes.
- A `json.Number` built by hand can carry non-JSON spellings that `strconv.ParseFloat` accepts (`json.Number("+1")` marshals as `1`); the raw JSON path rejects them first. Only a caller that skips the decoders can reach it.
