# Findings

## canonicaljson (about 900 lines, five files)

- One flat package mixes the public digest API with the encoder, the number
  formatter, the string rules and the strict validator; only eight symbols are
  public, everything else is private mechanism.
- Domain vocabulary (`Domain` constants, the `sha256:` reference format) and
  mechanism share files.
- Dead and duplicated code was already removed (`MarshalString`, `DomainTest`,
  fourteen hand-written digest strings); `deadcode` is clean.
- No `UBIQUITOUS_LANGUAGE.md`.

## ids (reviewed, not migrated)

An 80-line foundation: prefixes, the `Generator` contract and two generators. It was cleaned
(`Sequence` and unused prefixes removed). The owner decided it needs no layering; the deterministic
packages still may not import it because it holds the random generator (follow-up 9).

## Common

- `canonicaljson` sits at layer 0 with 25 importers; layering it moves about 30 layer table
  entries up one level.
