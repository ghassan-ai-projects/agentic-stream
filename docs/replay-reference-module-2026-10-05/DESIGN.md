# Design

Replay is a composition module like runtime: it drives lower modules
(spec, ingress, engine, episodes, qualification) inside a database it owns for
the duration of one bounded verification run. It gets runtime's layer set minus
a composition layer — replay's assembly is three constructor calls, so a
separate composition package would be a wrapper that only renames app.

## Layers

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade `internal/replay` | Public contract aliases (`Result`, `Mode`, `Capabilities`, ports, errors), `Run`/`RunMode`/`RunNTimes`/`AllHashesEqual`/`NewDeterministicBaseline` delegating one line each | Contain logic, SQL, file handles or transactions |
| App `internal/replay/internal/app` | Replay sessions: compile spec → save deployment → derive epoch → ingest → run engine → materialize episodes → collect result; mode dispatch and capability phases (recorded, shadow, counterfactual) | Import `database/sql`, `internal/storage`, `net`, or open files |
| Domain `internal/replay/internal/domain` | Vocabulary (modes, capabilities, worklist, trials, comparisons) and every verification rule as pure functions: capability admission, recorded-entry verification and matching, shadow binding/validation precedence, comparison sealing, admission windows, epoch selection, baseline policy, version hashing | Any I/O, clock reads, imports of `storage`, `spec`, `policy`, `episodes`, `eventlog` |
| Store `internal/replay/internal/store` | All SQL against the isolated database: worklist, version digests, snapshot digests and snapshots; the episode-materialization transaction through `episodes.Assembler`; comparison persistence through `qualification.ShadowComparisonStore`; spec deployment save | Decide admission, verify digests, or own another module's lifecycle semantics |
| Transport `internal/replay/internal/transport` | Trace file reading, isolated database creation, JSONL trace ingestion through the ingress adapter | Sequence a session or validate evidence |

Dependency levels: domain 3 (uses `canonicaljson`, `contractsv1`,
`decisions`), transport 7, store 8, app 9, facade 10 — strictly downward,
registered in `packageLayers` and `allowedImports`; `forbiddenImports` blocks
`internal/actions` and `internal/runtime` for every replay layer.

Cross-module derived values enter domain as parameters: app compiles the intent
catalog (`episodes`, `decisions`) and the policy digest (`policy`) and hands
them to domain validation rules; app converts `spec.Intent` values into domain
baseline intents so domain never imports `spec`.

## Public API (unchanged)

| Symbol | Kind |
| --- | --- |
| `Run(ctx, dbPath, specPath, tracePath, tenantID)` | deterministic session |
| `RunMode(ctx, mode, dbPath, specPath, tracePath, tenantID, capabilities...)` | mode session with explicit capabilities |
| `RunNTimes(ctx, specPath, tracePath, tenantID, n)` | determinism proof loop |
| `AllHashesEqual(results)` | pure check |
| `NewDeterministicBaseline(compiled)` | default baseline executor |
| `Result`, `Finding`, `ShadowComparisonResult` | reports |
| `Mode` + four constants, `ErrModeCapabilityRequired`, `ErrUnsupportedMode` | mode contract |
| `RecordedEntry`, `ReplayEpisode`, `RecordedLedger`, `RecordedLedgerForReplay` | recorded contract |
| `ShadowInput`, `ShadowOutput`, `ShadowExecutor`, `BaselineExecutor` | shadow contract |
| `SimulatedCommand`, `Simulator` | counterfactual contract |
| `Capabilities` | capability set |

## Rules every operation follows

1. A session opens a fresh isolated database; an existing file or WAL sidecar
   fails the run (`storage.OpenFresh` semantics preserved).
2. The virtual clock starts at the earliest valid trace processing time
   (Unix origin when none); invalid lines can never move it because ingress
   validity is checked before a line counts.
3. Deterministic mode builds the stream engine only; worker-aware modes build
   the full engine and materialize episodes exactly once, in one transaction.
4. Worker-aware modes fail closed without their explicit capability
   (`ErrModeCapabilityRequired`), before any engine or database work beyond
   mode validation ordering preserved from `RunMode`.
5. Recorded entries are verified (completeness → provenance digest → canonical
   schema-valid decision digest) and matched one-to-one against the worklist;
   duplicates, extras and misses are errors.
6. Shadow trials execute baseline and Tamoz on identical cloned snapshots,
   validate both decisions through the decision catalog, and persist exactly
   one sealed comparison per trial; differences become findings, never intents.
7. Counterfactual commands go to the simulator only; duplicates are rejected
   after the prior simulation was retained.
8. `EffectsAllowed` is false in every result; no mode gains a credential,
   resolver or effector input.

## Enforcement

| Guarantee | Gate |
| --- | --- |
| Downward-only imports per layer | `TestImportsOnlyPointToLowerArchitectureLayers`, `TestPackageLayering`, `TestAllowedImportsHaveNoStaleEdges` |
| Domain purity (no I/O, no clock) | `TestDomainPackagesArePure` |
| App infrastructure isolation | `TestApplicationLayersDoNotTouchInfrastructure` |
| SQL only in store | `TestModuleSQLStaysInStore` |
| Replay never reaches effects/runtime | `TestReasoningAndReplayCannotReachEffectImplementations` (extended to the new layers), `forbiddenImports` |
| Single durable owners unchanged | `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` (replay writes only through owner APIs) |
| Behavior preserved | Untouched facade golden/thermal tests plus moved boundary tests |

## Schema / wire changes

None. No migrations, no protocol changes, no digest or canonical-bytes
changes; `shadow_comparisons` records are produced by the same owner store with
identical field values.
