# One architecture for every package, and where small packages belong

Status: proposal, nothing implemented. Evidence: production lines and importing
modules per package at commit `fc2e42e`.

## The rule

Every module is a facade (the package root) over `internal/domain` plus the other
layers its work needs, with a `UBIQUITOUS_LANGUAGE.md` and a record under `docs/`.

| The module... | Layers beyond domain |
| --- | --- |
| owns durable tables | `app`, `store` |
| talks to an external system | `app`, `transport` (and `wire` for a protocol) |
| is pure rules | none: facade + `domain` |

The facade exposes only what other packages use. Exported code nothing outside the module uses stays inside `domain` and is listed in [EXPOSURE.md](EXPOSURE.md) as a candidate for deletion, for the owner to decide.

A package under about 300 production lines does not stand alone: it merges into
the module whose vocabulary it belongs to. The only stand-alone small packages
allowed are listed under "Exceptions", each with the dependency reason. Test
support lives under `internal/testsupport/` and is exempt from layering.

## Where each unlayered package goes

| Package | Lines | Importers | Decision | Home |
| --- | --- | --- | --- | --- |
| `duration` | 43 | 6 | merge | `spec` as `spec.ParseDuration`; every importer but `episodes/domain/budget.go` already imports `spec` |
| `eventschema` | 183 | 3 | merge | `spec`: the registry is spec vocabulary, and `Register` has one production caller, `spec` deployment. The four test seeders call a new exported `spec.RegisterEventSchema`, which deployment also uses. This reverses the earlier "dropped" note |
| `ids` (prefixes) | 74 | 33 | split | prefix constants to `contractsv1`, so deterministic packages can share them (closes follow-up 9) |
| `ids` (generators), `clock` | 74, 156 | 33, 26 | merge | one package for the injected sources of non-determinism, time and randomness (name open) |
| `interlock` | 61 | 11 | merge | `actionport`: it is the action plane's readiness boundary and policy, actions and watch all read it. `control` stays rejected because it would widen their imports |
| `actionport` | 87 | 26 | keep, absorbs `interlock` | facade + `domain` (contracts) + `store` (interlock table) |
| `executor/conformance` | 81 | 0 prod | move | `testsupport/executorconformance` |
| `executor/fixture` | 124 | 1 | decide | see open question 3 |
| `telemetry` | 480 | 22 | migrate | facade + `domain` (counters, latency) + `transport` (OpenTelemetry, Prometheus handler) |
| `storage` | 333 | 48 | exception | it is the database adapter itself; the shape would wrap one adapter in another |
| `contractsv1` | 580 | 69 | migrate | facade + `domain`; absorbs the id prefixes |
| `worker` | 685 | 5 | migrate | facade + `domain` (constants, budget validation) + `transport` (sockets, reference server) |
| `api` | 705 | 3 | migrate | facade + `domain` (selection, problem) + `app` (SSE delivery) + `transport` (HTTP) |
| `operators` | 1011 | 9 | migrate | facade + `domain` |
| `situations` | 719 | 16 | migrate | facade + `domain` |
| `spec` | 1014+ | 34 | migrate | facade + `domain` (compiler, CEL, references, durations) + `store` (deployments, event schemas) |

Already uniform: the 21 layered modules plus `canonicaljson` and `decisions`.

## Exceptions (stand-alone, no layers)

- `storage`: the database adapter itself.
- `testsupport/*`: test helpers.
- `actionport`: an 87-line contract package (the effect port). Merging it into `actions` would make `device` and `watch` import the action plane to implement `Effector`, which the forbidden-import invariants rule out. It stays a port.
- `interlock`: a 61-line SQL-bound port that no contract package, and no module without widening dependencies, can host.

## Order

1. Merges with no new layers: `duration` and `eventschema` into `spec`, `interlock`
   into `actionport`, `ids`+`clock`, conformance to `testsupport`.
2. Pure modules to facade + `domain`: `contractsv1`, `operators`, `situations`.
3. Layer `spec`, `worker`, `telemetry`, `api`.
4. Gates: one table-driven gate that lists every package under `internal/` and
   requires facade + `internal/domain` unless it is an exception.

Each step is its own commit with focused tests, lint and the architecture gates.

## Costs to know

Merging `ids`+`clock` touches about 60 files; `duration` and `eventschema` about 10;
`interlock` about 35. These are import-path edits, not logic changes. Layer-table
renumbering may be needed again for `actionport` (it gains a store).

## Progress

| Step | Status |
| --- | --- |
| `duration` and `eventschema` merged into `spec` (`spec.ParseDuration`, `spec.EventSchema`, `spec.RegisterEventSchema`; `event_schemas` is now owned by `spec`; data file `internal/spec/event_schema_data.json`) | Done |
| `ids` + `clock` merged into `internal/sources`. The id prefixes stay beside the generators (not `contractsv1`: the action plane may not import it). The old import ban on clock for deterministic layers became a symbol-level gate: domain layers and the replay store may use `sources.Prefix*` and `sources.Deterministic` but never `Clock`, `Physical`, `Virtual`, `Random` (`TestDeterministicLayersDoNotUseTimeOrRandomSources`, proven by injection) | Done |
| `interlock` into `actionport` | **Tried and reverted.** `actionport` is a contract package that must not import `database/sql` (the interlock reader is defined on `*sql.Tx`), and its layer would sit level with its own consumers. `control` stays rejected (it widens the imports of policy, actions and watch). Result: `interlock` stays a documented stand-alone exception unless you choose `control` |
| `executor/conformance` to `internal/testsupport/executorconformance` | Done |
| `executor/fixture` to `testsupport` | Held: composition imports it for demo mode, so it is production code; moving it needs a decision on demo mode |
| `contractsv1` to facade (production-used surface only) + domain, with its schemas and conformance data under `internal/domain`; see [EXPOSURE.md](EXPOSURE.md) | Done |
| `telemetry`, `worker`, `api`, `spec` layered (facade + domain + transport/store/app as each needs); `spec` now owns `spec_deployments` and `event_schemas` through `spec/internal/store` | Done |
| `operators`, `situations` to facade + domain | Done |
| Rename `contractsv1` to `contracts` (drop the version from the package name; the version stays in `ContractVersion`, `schemas/v1` and the wire ids) | Proposed, awaiting your decision |
| One gate requiring facade + `internal/domain` for every module | Not started |
