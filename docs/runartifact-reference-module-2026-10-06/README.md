# Run artifact reference module (2026-10-06)

`internal/runartifact` (with `soak` merged in, commit `478882b`) is now a
facade over `internal/app` (export and verify use cases), pure
`internal/domain` (manifest, ledger encoding, checksum and binding rules, soak
verdict, every verification rule), read-only `internal/store` (one snapshot
transaction, including the device authority safety read) and
`internal/transport` (artifact directory: reserve, atomic publish, read). The
module owns no tables. Vocabulary: [UBIQUITOUS_LANGUAGE.md](../../internal/runartifact/UBIQUITOUS_LANGUAGE.md).

Layers: domain 2, transport 3, store 8, app 9, facade 10. The policy module's
pure digest and definition functions reach the domain as function values
(`domain.PolicyDocuments`) supplied by the facade, so no lower layer imports
`policy`.

## Behavior that must not change

Exported file set and bytes, checksum index format, verification order
(integrity, then JSON syntax, then manifest bindings, then ledger digests),
error texts, the snapshot's single read-only transaction, never overwriting an
existing output.

## Deliberate behavior changes

- A database error while reading the latest spec digest, policy digest or device
  state now fails the export; before, every error there was swallowed as "absent"
  (only no-row is absent now).
- `KnownBlindSpots` supplied by the caller is sorted on a copy; before, the
  caller's slice was sorted in place.
- The global (tenant-less) soak report path is gone (see the remaining-migration plan).

## Gates

Layer table and allowed imports, repository map, facade delegation, opaque
snapshot and application ports (`architecture_runartifact_test.go`), and the
SQL-in-store gate, which now covers this module.

## Rating

Layering 9, domain rules 9, fail-closed safety 8, ubiquitous language 8, tests 8,
data-level encapsulation 7, type safety 7, simplicity 8. Weaknesses in order:
the store reads tables it does not own (`event_log`, `commands`, `decisions`,
`policy_evaluations`, ...), which waits for the owner read ports; ledger rows are
untyped `[]any` values; the facade still exposes free functions rather than a
configured service.
