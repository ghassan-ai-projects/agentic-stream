# Code reachability audit

Commands (using the repository-pinned x/tools version):

```sh
go run golang.org/x/tools/cmd/deadcode ./cmd/...
go run golang.org/x/tools/cmd/deadcode -test ./...
```

The production entry-point scan sees only code reachable from the CLI; it is
not a declaration that every other production API is dead. Compare it with
source consumers and the test-aware scan. The test-aware scan finishes with
no unreachable functions anywhere in the repository after this cleanup.

| Candidate | Evidence | Decision |
| --- | --- | --- |
| Public `Service.Rebind` and app service forwarder | Reported unreachable from CLI; source callers were facade tests only; runner calls its private assembler directly | Remove both public forwarders; keep private rebind use case and its behavioral tests |
| Public Assembler/Runner constructors and setters | Runtime now configures one service; remaining legacy builder calls are layer tests | Remove from production; retain only used isolated constructors/options in `app/test_config_test.go` |
| `Assembler.WithCostControl`, `Runner.WithCostControl`, `Runner.WithTelemetry` test helpers | Test-aware deadcode scan reports all three unreachable | Delete them |
| `NewRunnerWithEpoch` test constructor forwarder | Only called by another test constructor | Collapse to one isolated test runner constructor |
| `FakeExecutor` in episode app/facade | Live demo composition needs deterministic reasoning; not dead or test-only | Move to `executor/fixture`; update all consumers; conformance and determinism tests pass; 90% coverage |
| Public `CompileIntentCatalog` | Unreachable from CLI, but production source consumers exist in replay shadow validation and executor conformance; domain compilation is used by live assembly | Keep facade delegation; do not remove another module's replay contract during this migration |
| `store.RowQueryer` | One transaction caller, exposed raw SQL to use cases | Remove; lifecycle read takes opaque store Tx |
| Domain `NullString.Valid` | Scanned but never read; only `.String` entered the reconsideration document | Replace with the actual reconciliation-status string; SQL null decoding remains in store |
| `SnapshotEvidence.JSON` | Retained after validation but only read by a test | Remove; validation consumes raw bytes and stores the validated document, entity and digest; raw RequestJSON remains unchanged |
| Exported private document/risk helpers | Used only within domain or its tests | Make private; retain tested pure behavior |
| Mutable risk-rank map | Internal lookup only | Replace with a pure private function |

Useful test-only support stays in `_test.go`: real database fixtures, isolated
runner/assembler construction, explicit test fences, shadow-store and rebind
setup. Integration tests use the actual fixture adapter from `app_test`, so
there is no copied deterministic executor or epoch-refusal implementation.

The production scan also reports unrelated existing APIs in authority,
contracts, engine, eventlog, native executor, replay, qualification and other
modules. Those are outside the requested episode migration and need their own
consumer/contract review before deletion. No unrelated functions were removed.
