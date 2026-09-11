# A-045 · `internal/runartifact/export_verify.go`

LOC: 200 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Verification cannot be satisfied by non-regular (symlinked) artifact members (P2: immutable, self-contained evidence) — FAIL.
- Checksum parsing rejects traversal, duplicates, malformed lines (P2) — pass.
- Required files, checksums, canonical JSON/JSONL all enforced; manifest schema version pinned (P1/P5) — pass.
- No duplicated logic vs the writer side (P3) — pass (`verifyManifestBindings` reused).
- Errors wrapped with file context (P1) — pass.

## Findings
- **LOW F1. Symlinked artifact members pass verification** — `internal/runartifact/export_verify.go:112-128,158-160`. `verifyDirectoryContents` checks entry names only and `readArtifactFile` follows symlinks, so a member that is a symlink pointing outside the artifact verifies as long as its target content matches `checksums.sha256`. The verified result then depends on external mutable state, breaking the self-contained immutable-artifact claim documented in `export.go:69-70` (re-pointing the symlink after Verify changes content without any checksum mismatch). Fix: reject non-regular entries (`entry.Type() != 0` in the directory walk, `os.Lstat` in `readArtifactFile`).

## Checked, not an issue
- P2: checksum lines validated against path traversal (`.`/`..`, separators, backslash, non-basename) and duplicates (`:76-84`); hex format and length enforced.
- P1: every checksummed file is re-hashed and compared (`:99-108`); required-file list enforced for all JSONL+JSON names (`:93-98`); unexpected directory entries rejected (`:122-126`); canonical JSON and JSONL canonicality enforced (`:162-200`); manifest schema version pinned (`:152-154`).
- P3: `Verify` reuses the writer-side `verifyManifestBindings` and the shared `verifyJSONL`/`verifyLedgerFiles` — no divergent duplicates.
- P5: scanner buffers sized via `maxJSONLLineBytes`; deterministic file-list handling.
