# Validation and review

## Gates

| Check | Result |
| --- | --- |
| Focused evidence/remote/runtime/CLI uncached race | Pass |
| Whole-tree golangci-lint | Pass; zero issues |
| Root architecture gates | Pass |
| Eight injected violations | All rejected by named gates; fixtures removed and source restored |
| Pinned test-aware deadcode | Pass; zero unreachable functions |
| Production CLI deadcode | No evidence candidates; unrelated modules left unchanged |
| Full make ci-check | Pass; every package meets the 60% coverage gate |
| Full non-short go test -race -count=1 ./... | Pass |
| git diff --check | Pass |

Injected violations: facade logic, domain I/O, SQL outside store, raw application
SQL port, application protocol import, domain codec import, transaction alias,
and public database handle. Both full gates completed successfully. Task-local caches avoid sandbox cache failures; local Unix socket
tests require approved execution. No dependency, schema or threshold changes.

## Review

Checked envelope/trace/token/scope/range/argument/deadline error precedence;
reservation identity before result integrity; exact token claim and fingerprint
bytes; unchanged event result JSON; clock reads; owner assertions on the exact
transaction; leased/fenced completion predicates; detached five-second failure/
completion contexts; current-attempt checks before reservation and completion;
durable replay and terminal failure refusal; startup recovery rollback.

Deliberate changes are listed in PLAN.md. Every production worker query uses the
durable path. The facade delegates and has no mutable key/database fields;
app/domain/store import no protobuf/gRPC or codecs. Domain rules read no clock.
Owner callbacks remain explicit composition inputs; presence validation cannot
prove an externally supplied assertion is correct.

## Ratings

| Dimension | Score / 10 | Evidence and remaining weakness |
| --- | --- | --- |
| Layering | 10 | Enforced facade, app, pure domain, opaque store, wire and transport; injected gate proofs |
| Domain rules | 9 | Scope, lifetime, authorization, attempts, identity/integrity and bounds are pure; persisted lifecycle strings remain shared vocabulary |
| Fail-closed safety | 9 | Required keys, ownership and durable call ports; construction refusal, live attempt checks and safe errors; composition must supply a real owner assertion |
| Ubiquitous language | 9 | Scope, Call, Reservation, QueryResult and recovery reasons match durable/wire names |
| Tests | 9 | Golden bytes, ownership, supersession, cancellation, concurrency, replay and rollback regressions; full checks recorded above |
| Data encapsulation | 8 | Private copied keys and opaque handles; existing transactional reads of episode/attempt tables remain |
| Type safety | 9 | Closed argument/result envelopes and transport-neutral errors; schema-owned event Data remains a map |
| Simplicity | 8 | One configured facade; explicit capability/ledger/call composition reflects separate startup dependencies; six packages add navigation cost |

## Ordered future work

1. Introduce owner-provided transactional episode/attempt read ports without
   changing reservation/completion ordering or splitting transactions.
2. Bound provider result construction before materializing large event windows.
   Current row bounds apply during reads; total byte bounds are enforced after
   encoding, so oversized source rows can temporarily consume excess memory.
   Preserve the current error contract and exact bytes for successful results.
3. Add focused storage/codec I/O fault tests where they can demonstrate currently
   uncovered branches rather than mirror implementation.

These are future improvements, not incomplete facade/layer work. Full checks
support migration correctness; deployment qualification remains separate.

## Final coverage

| Layer | Statement coverage |
| --- | --- |
| Facade | 100.0% |
| App | 81.6% |
| Domain | 95.6% |
| Store | 84.1% |
| Transport | 94.3% |
| Wire | 83.5% |

The optional installed deadcode target was unavailable; the repository-pinned
Go tool was run directly, including its test-aware mode. Optional govulncheck
and pre-commit were unavailable. No required CI check remains blocked.
