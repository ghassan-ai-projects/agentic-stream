# kernel

Status: done
Round: 2

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/kernel` | 100% | 100% | 1.1 s | 1.1 s | 3 / 3 | 6 / 25 |

Time is dominated by the race-instrumented test binary start; no test takes more than a few milliseconds.

## Findings and changes

### Removed
- Nothing deleted; the three tests were restructured (below).

### Renamed or moved
- `TestDurableTimeTextOrdersChronologicallyAndRoundTrips`: split. The `ParseTime` rejection and the `FormatTime` literal moved to their own tests; the ordering and round-trip loop stays.

### Improved
- T4: `ParseTime`, `ParseStoredTime` and `DecodeDigest` rejections assert which rule fired (message content), not just `err != nil`.
- T11: every rejection is a named table row, one cause per row (digest: prefix, wrong algorithm, empty, upper case, not hex, odd length, short, long; stored time: six non-durable layouts).
- T6: every subtest is parallel.

### Added
- `TestFormatTimeIsUTCWithANineDigitFraction`: nine-digit fraction, UTC conversion from another zone.
- `TestParseTimeReadsAnyRFC3339TextIntoUTC`: with and without a fraction, offsets normalized to UTC.
- `TestParseStoredTimeAcceptsOnlyTheTextFormatTimeWrites` (extended): lowercase separators, offset form, short fraction; the "valid timestamp in another layout" message.
- `TestEncodeDigestPrefixesLowercaseHex` / `TestDecodeDigestRejectsNonCanonicalForms`: golden `sha256:ab00…cd`, length messages.

### Speed
- Nothing slow.

## Production code touched
- none

## Invariants proven here
- None directly. It owns the durable time and digest text rules every module shares (underpins 3 and 8). `TestKernelStaysPure` lives in `internal/architecture`.

## Open items
- none
