# DUP-018: The episode admission columns have four parallel struct and column-list views

- Status: partly fixed
- Severity: medium
- Verdict (finders): REAL
- Themes: persistence
- Wave: 3
- Finder sources: P7 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `episodeledger`. Add a column-parity test between the INSERT list and the read before changing anything.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P7: The episode admission record has four parallel struct/column-list views across episodeledger, episodes and replay

- Verdict: REAL
- Shared meaning: the admission-time columns of an `episodes` row (identity, executor name/version, model policy, prompt version, snapshot/prompt/objective digests, admission key, request JSON, dispatch policy, policy epoch).
- Sites:
  - internal/episodeledger/internal/store/admission.go:39-47 `INSERT INTO episodes (18 columns)`; domain Admission struct internal/episodeledger/internal/domain/admission.go:15-21.
  - internal/episodes/internal/store/dispatch.go:10-34 `DispatchedEpisode` struct + :59-66 `SELECT` of 17 of the same columns + Scan at :36-44.
  - internal/episodes/internal/domain/admission.go:34-38 `AdmittedEpisode` (Request -> Admission) and internal/episodes/internal/app/runner_claim.go:82-93 `episodeClaimFromDispatched` (DispatchedEpisode -> Request), with hex<->bytes digest round trips (`DecodeRequestDigests` / `EncodeDigest`).
  - internal/replay/internal/store/store.go:96-103 `SELECT executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, request_json FROM episodes WHERE episode_id=?` (partial view, hand-formats `sha256:`).
  - internal/episodeledger/internal/store/episode_reads.go:14-27 a fifth projection (`EpisodeRecord`) of overlapping columns.
- How they differ: same column set expressed as write struct, read struct, domain request and view; the dispatch query also embeds the control-owned `epoch_control` predicate (C2).
- Risk if left: adding a column to `episodes` (as 014, 024, 028 did) means editing migration + 6 Go places; an omitted column in the dispatch SELECT is silently zero-valued.
- Proposed canonical owner: `internal/episodeledger` (owner of `episodes` writes; episodes store already imports it).
- Proposed fix: episodeledger exposes `NextDispatchable(tx, tenant, killedSuperseded) (Admission, EpisodeMeta, error)` returning its own `Admission`; episodes drops `DispatchedEpisode` and its SELECT, converting `Admission -> Request` in one function that already exists in the opposite direction (`AdmittedEpisode`). Replay's `ShadowRequest` can use the same `Admission` accessor by id.
- Behaviour to preserve: dispatch order `accepted_at, episode_id`, `stale_rebind_count`, the killed-epoch superseded branch, admission_key bytes, ids.
- Verification: episodes/internal/store/store_test.go, transaction_test.go, episodeledger lifecycle tests; add a reflection/column-parity test between the INSERT column list and the Admission read.

## Outcome

Verified: confirmed. `episodes` re-listed the admission columns in a `DispatchedEpisode` struct, a 17-column SELECT and a Scan, and converted it to `Request` in `episodeClaimFromDispatched`; `replay` re-listed six of them and hand-formatted `sha256:`. The finder's "four views" were really: INSERT list (ledger), dispatch read (episodes), replay read, and `EpisodeRecord` (ledger inspection view). The dispatch SELECT's `epoch_control` read is a read of control's table, as the ledger already does for `runtime_owner`; no gate objects.

Changed:
- `internal/episodeledger/internal/store/admission.go`: one `admissionColumns` list with `admissionValues` and `admissionTargets`, used by the INSERT, the new `ReadAdmission`, and the dispatch read. `InsertEpisode` drops the redundant `dispatchPolicy` argument (always equal to `req.DispatchPolicy`).
- `internal/episodeledger/internal/store/dispatch.go` (new): `NextDispatchableEpisode` with the live/killed-unstarted predicates and order `accepted_at, episode_id` (moved from episodes); `domain.DispatchableEpisode{Admission, StaleRebindCount}`; facade `NextDispatchableEpisode(tx, ...)` and `ReadAdmission(db, id)` (`reads.go`, app `reads.go`).
- `internal/episodes/internal/store/dispatch.go` shrank to a delegating `DispatchableEpisode` returning `(episodeledger.DispatchableEpisode, found, error)`; `DispatchedEpisode`, its SQL and `store.ErrNoRows` are deleted. `internal/episodes/internal/domain/admission.go` gained `AdmittedRequest` (inverse of `AdmittedEpisode`); `app/runner_claim.go` uses it.
- `internal/replay/internal/store/store.go` `ShadowRequest` uses `ReadAdmission` and `canonicaljson.EncodeDigest`.
- README note in `internal/episodeledger/README.md`; `architecture_episodeledger_test.go` operations list.

Deferred: `EpisodeRecord` (`episode_reads.go`) keeps its own projection; it is an inspection view that adds lifecycle, attempts and timestamps, with COALESCE and a different shape, not the admission record. Error text of the dispatch read now reads "query admitted episode: read dispatchable episode: load dispatchable episode: ...".

Pinned by: `TestAdmissionColumnsRoundTripEveryField` (ledger store: reflection fills every `Admission` field, INSERT then `ReadAdmission` must return it unchanged, so a new field omitted from either list fails), `TestNextDispatchableEpisodeOrdersLiveEpisodesAndAdmitsKilledUnstarted` (order, stale_rebind_count, killed branch, tenant), `TestAdmissionRecordSurvivesTheRequestRoundTrip` (episodes domain: `AdmittedEpisode(AdmittedRequest(a)) == a` with every field non-zero), `TestDispatchableEpisodeReadsOldestAdmittedEpisode`, and the replay `recorded_ledger_test.go` ShadowRequest check.
