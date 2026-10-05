# Device authority as the reference module — October 2026

This folder is the working record for rebuilding `internal/authority` as the
reference implementation that other runtime modules follow. Baseline: `6750c10`
on branch `general-improvements-1`. Backward compatibility is not a goal: the
public API, call sites and the table schema may change.

- [Findings](FINDINGS.md): what is wrong with the package today, with evidence.
- [Ubiquitous language](UBIQUITOUS_LANGUAGE.md): the shared vocabulary of the
  device-authority context, the words it retires, and where each term lives in
  code and storage.
- [Target design](DESIGN.md): layers, public surface, transaction and clock
  rules, and the checks that keep the layers honest.
- [Module pattern](MODULE_PATTERN.md): the reusable recipe for applying the same
  structure to another package.
- [Plan](PLAN.md): rounds, verification, and deferred follow-ups.

## Summary

The package mixes three things inside the same functions: the device-authority
rules (claims, fences, reconciliation, safe-stop latching), SQL, and transaction
orchestration. Its public surface is three loosely related structs with
exported fields, so callers build half-configured values and reach into their
database handles. Several rules exist twice, once in Go and once in SQL, and
the vocabulary drifts between "authority epoch" and "owner epoch", "device
lifetime" and "boot", "require" and "required".

The target is one module with three enforced layers:

```
internal/authority/                 public API + application services (transactions, admission)
internal/authority/internal/domain/ pure vocabulary and rules (no I/O, no clock reads)
internal/authority/internal/store/  every SQL statement for the module's tables
```

Go's `internal/` directory makes `domain` and `store` invisible outside the
module. Architecture tests make `domain` pure and make `store` the only writer
of the module's tables.
