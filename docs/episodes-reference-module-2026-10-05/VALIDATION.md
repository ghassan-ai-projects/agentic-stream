# Validation and review

## Final checks

| Check | Result |
| --- | --- |
| `make ci-check` | Pass, including pinned protoc 35.1, tidy, build, vet, zero-issue lint, short uncached race coverage and docs |
| `go test -race -count=1 ./...` | Pass; full non-short, uncached suite, including worker and runtime integration tests |
| Pinned `go run golang.org/x/tools/cmd/deadcode -test ./...` | Pass; zero unreachable functions in the entire test-aware graph |
| Production CLI reachability | Episode catalog facade is the only retained episode candidate; replay/conformance source consumers justify it; see CODE_AUDIT.md |
| Seven injected architecture violations | Each rejected by its named gate; fixtures removed; clean gates pass |
| Public documentation | 68 pages and volatile surfaces verified; moved source/test links repaired |
| `git diff --check` | Pass |

Make's deadcode binary is not installed locally; the pinned module tool was
run directly instead. Optional govulncheck is not installed and was skipped
by Make. Pre-commit is not installed. No dependencies or thresholds changed.
The sandbox initially prevented cache access and Unix sockets; validation
used a task-local cache and approved execution for local socket tests.

## Race coverage of the changed episode boundary

| Package | Statement coverage |
| --- | --- |
| `episodes` facade | 100.0% |
| `episodes/internal/app` | 64.8% |
| `episodes/internal/domain` | 79.8% |
| `episodes/internal/store` | 76.3% |
| `executor/fixture` | 90.0% |

Every repository package with non-generated statements meets the 60% floor.
Existing replay goldens and the aquaculture catalog digest pin are unchanged.

## Self-review

The facade contains only contract aliases, construction adaptation and direct
service delegation. Every transaction remains owned by its original caller or
runner step; private store wrappers expose no raw database/transaction fields.
Ledger mutation owners and intent-producer permissions remain unchanged.
Pure rules take loaded values and time; they have no I/O or clock reads.

Checked ID/digest preparation order, error precedence, wrapped sentinel causes,
clock reads, stale/retry budgets, trace/budget/entity hydration order, joined
rollback, epoch gates before and after execution, detached conclusion context,
supersession cancellation and shadow isolation. No protocol/schema/golden or
production credential/effect authority changed. Runtime and replay consumers
use the new service; fixture execution remains available in demo composition.

## Ratings

| Dimension | Score / 10 | Evidence and remaining weakness |
| --- | --- | --- |
| Layering | 10 | Public facade, private use cases, pure rules and opaque store are enforced; concrete fixture is outside lifecycle |
| Domain rules | 9 | Assembly, validation/storage, freshness, retry, epoch, failure and shadow rules are pure; some canonical document construction remains map-based |
| Fail-closed safety | 9 | Execution ports required at construction; assembly-only refuses RunOnce; live fences retained; explicit unowned fixture checks remain a composition assumption |
| Ubiquitous language | 9 | Public Service and admission Episodes dependency match their roles; persisted reason/status vocabulary remains stable |
| Tests | 9 | Full CI race coverage, rollback, error precedence, conformance and injected gate proofs; application error branches have the lowest coverage |
| Data encapsulation | 8 | Transactions and database handles are private; existing cross-module read-only SQL projections remain in store |
| Type safety | 8 | Typed requests/outcomes/records and provenance eliminate unchecked casts; canonical schema-driven documents still use maps |
| Simplicity | 9 | One configured public service and three operations; no compatibility wrappers, copied test executor, new dependencies or extra production layers |

Highest-value follow-ups, outside this migration: replace cross-module read
projections with owner-provided ports as those modules are migrated; introduce
closed typed projections for canonical documents without changing their bytes;
add targeted application failure tests beyond the existing coverage floor.
These do not block the facade/layer migration or its current correctness gates.
