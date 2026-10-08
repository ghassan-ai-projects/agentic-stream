# U07 — Remove test-only facade exports

Status: todo · Decision: **delete or move, per row** · Priority: P2 · Size: M

## Finding and decision per symbol

| Symbol | Test callers | Decision | Reason |
| --- | --- | --- | --- |
| `authority.ParseReconciliationEvidence`, `authority.PhysicalEvidenceComplete` | device gateway test, authority service test | **Delete** | Wrappers over domain helpers. The device test builds evidence through the facade's real operations; the domain test calls the domain directly. |
| `api.NewHealthHandler`, `api.NewSSEHandler` | api, transport, cmd and policy tests | **Delete** | Production composes `api.NewRuntimeHandler`. Out-of-package tests use it (with nil components where allowed); transport tests call the transport functions. Tests then cover the handler production serves. |
| `control.SetCostLimit` | runtime admission and three control tests | **Delete** | Production writes limits through `ApplyCostCeilings`; tests use it too, which is better evidence. If a test needs a value `ApplyCostCeilings` cannot express, add a `controltest` helper instead of a production export. |
| `episodes.CompileIntentCatalog` | episodes facade, remote executor tests; `testsupport/executorconformance` | **Move** to `internal/episodes/episodestest` | Tests need the exact catalog episodes assembly produces. A `*test` subpackage can import `episodes/internal/domain` and keeps the export out of production. |
| `spec.EventSchemaJSON`, `spec.RegisterEventSchema` | eventlog, ingress tests | **Move** to `internal/spec/spectest` | Schema-seeding helpers for other modules' tests. |
| `contractsv1.ConformanceValidFrame` and the domain `Conformance*` loaders | contractsv1 domain test, device UDS test | **Move** to `internal/contractsv1/contractstest` | Fixture loaders for committed JSON frames. The frames stay where they are (domain data stays in JSON). |

## Rule this establishes

Production packages contain no exported function that only tests call. Test
helpers that need module internals live in a `<module>test` subpackage inside
the module, which Go's `internal` rule allows. The architecture gates (and U12)
treat `*test` packages like `internal/testsupport/`.

## Experiment constraint

Only the Go loaders move. The JSON files under
`internal/contractsv1/internal/domain/conformance/v1/`, including
`thermal-capability-catalog.json`, stay at their current path: the experiment
runbook passes that path to `serve --device-catalog`, and X01 pins it.

## Done when

- 15 symbols are gone from `deadcode ./...`.
- `architecture_episodes_test.go` and `architecture_decisions_test.go`, which
  name `CompileIntentCatalog`, are updated.
- `TestAquacultureIntentCatalogDigestParity` still passes unchanged.
