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

## ids (about 80 lines)

- One file holds the identity-space prefixes, the `Generator` contract, the
  random generator (entropy from `crypto/rand`) and the deterministic one.
- Deterministic-only packages (replay domain, native executor, ledger domains)
  may not import it because it also holds the random generator; a split puts the
  prefixes and the deterministic generator in a pure domain and the random source
  in an adapter. The architecture gates still forbid those imports of the facade.
- No `UBIQUITOUS_LANGUAGE.md`.

## Common

- Both sit at layer 0 with 25 and 16 importers; layering them moves 34 layer
  table entries up one level.
