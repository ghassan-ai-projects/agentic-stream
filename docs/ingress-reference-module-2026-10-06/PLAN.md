# Ingress migration plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Domain, store, transport, app and facade together (ownership gate); move the embedded simulator data with domain; update runtime and replay callers | Domain, store, transport, app and facade tests; runtime and replay suites | Pending |
| 2 | Architecture gates, injection proof, guide, maps, validation | Injected failures, full CI, race | Pending |

## Behavior that must not change

Admission order (decode, envelope contract, schema); quarantine identities (`<connector>:line:<n>`, live `live-uds:<instance>:<conn>:<n>`); the 64 KiB line bound; resync after an oversized file line and disconnect after an oversized socket line; batch size 100; the simulator grammar and its error texts; the client limit, queue size and shutdown ordering of the live socket; socket path safety and owner-only permissions.

## Deliberate changes

- A storage error while reading a simulator checkpoint is returned, not treated as "start at line 0".
- Both checkpoint writers share one upsert and one JSON layout; the simulator's key order changes, readers are tolerant.
- Simulator replay uses the service clock (physical by default); tenant defaults to `default` for every source.
- `New(Config)` replaces the three constructors and the `With*` setters.
- The embedded channel-field data moves to `internal/ingress/internal/domain/simulator_data.json`; documentation references are updated.

## Deferred

Typed simulator records instead of `map[string]any`; one transaction spanning the batch and its checkpoint.
