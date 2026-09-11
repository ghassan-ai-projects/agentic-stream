# A-061 · `internal/runartifact/export.go`

LOC: 158 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Single source of truth for the exported file set (P3) — FAIL: name lists duplicate the snapshot writer.
- "Never overwrites" contract is race-free (P2) — FAIL: stat-then-rename TOCTOU can replace an empty directory.
- Atomic publish with cleanup, safe file permissions, validated names (P2) — pass.
- Exported symbols documented; context first param (P5) — pass.

## Findings
- **MED F1. Two sources of truth for the artifact file set** — `internal/runartifact/export.go:77-85` vs `internal/runartifact/export_snapshot.go:38-59,69-93`. `exportedJSONL`/`exportedJSON` hand-maintain the same names that `exportLedgerFiles`' query map and the explicit `files[...]` writes produce. Adding or renaming a ledger requires synchronized edits in two files; when they drift, `Verify`'s required-file list silently lags the published set (a published file no longer required, or a required name nothing produces — the latter fails loudly only at verify time). Fix: derive the required-name list from the file map returned by `snapshot` (persist it once, e.g. into the manifest or checksums header) instead of a parallel literal.
- **LOW F2. TOCTOU can violate the no-overwrite contract** — `internal/runartifact/export.go:119-131,154`. `ensureOutputIsAvailable` stats, then `publishArtifact` renames later; on Unix `rename(2)` succeeds when the target is an existing empty directory, so a directory created in the window is silently replaced, contradicting the documented contract (`:69-70` "Export never overwrites an existing directory"). Fix: re-stat after rename and fail, or create the final directory exclusively first and publish into it.

## Checked, not an issue
- P2: publish is atomic (`MkdirTemp` + `rename`) with `defer os.RemoveAll(tmp)` cleanup; temp dir 0700, files 0600; `writeFile` rejects non-basename/separated names.
- P1: errors wrapped with operation context; no swallowed errors; ctx passed to the snapshot.
- P4/P5: package doc states the evidence-projection scope; `Manifest`/`DeviceIdentity`/`WorkerMetadata`/`Options`/`Export` documented; no transport or domain leakage.
- `checksumFile` sorts names, so `checksums.sha256` is byte-reproducible across runs.
