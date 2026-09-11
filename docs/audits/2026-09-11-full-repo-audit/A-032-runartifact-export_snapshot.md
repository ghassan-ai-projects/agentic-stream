# A-032 · `internal/runartifact/export_snapshot.go`

LOC: 286 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Errors wrapped %w, none swallowed (P1) — FAIL: three enrichment paths swallow DB/decode errors.
- BLOB representation is unambiguous (P1/P5) — FAIL: JSON-validity sniffing silently re-types values.
- Consistent single snapshot view, canonical JSON for all exported files (P5) — pass.
- Tenant scoping correct for every query (P2) — pass.
- Errors wrapped elsewhere (P1) — pass.

## Findings
- **MED F1. Swallowed errors in manifest/device enrichment** — `internal/runartifact/export_snapshot.go:122-126,129-131,152-158`. The `SpecDigest` lookup treats any Scan error (including real DB failures — corruption, missing migration, locked DB) as "digest not provided"; the `PolicyDigest` lookup discards the error entirely (`_ = tx.QueryRowContext(...).Scan(...)`); `enrichDeviceIdentity` returns the unenriched manifest on both Scan and `json.Unmarshal` errors. Consequence: a genuinely failing DB silently produces a manifest with empty digests, and `verifyManifestBindings` (`:257-286`) skips empty digests — the artifact ships with silently weaker provenance and no failure signal. Fix: tolerate only `errors.Is(err, sql.ErrNoRows)`; wrap and return everything else.
- **LOW F2. Heuristic BLOB re-typing** — `internal/runartifact/export_snapshot.go:211-223`. `databaseValue` inlines any `[]byte` that is valid JSON: a BLOB whose bytes happen to be e.g. `123`, `true`, or `"x"` is exported as a JSON scalar instead of its base64 encoding, silently changing the exported representation depending on content. Acceptable for known JSON columns, but the heuristic has no guard for scalar-valued binaries. Fix: restrict decoding to the known JSON document columns or require the decoded value to be an object/array.

## Checked, not an issue
- P1: one consistent read via `BeginTx(ReadOnly)`; all other errors `%w`-wrapped with operation context; `rows.Err()` checked.
- P2/P4: `authority-events.jsonl`/`safety-events.jsonl` without tenant filter is correct — the tables have no `tenant_id` column (`migrations/030_authority_reconciliation_soak.sql:35,74`); device-state disambiguation refuses to guess when multiple device rows exist (`:147-149`).
- P5: every exported JSON file is canonical JSON with trailing newline (`canonicalJSONFile`); canonicalization verified against stored digests; policy digest cross-checked against `policy.DigestForVersion` before export (`:247-253`).
