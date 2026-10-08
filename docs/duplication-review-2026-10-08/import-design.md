# Import design review (2026-10-09)

Status: done. This note records why the duplication fixes needed so many
new `allowedImports` edges and layer moves, and the design that removes the need.

## What happened

The owner proposed a kernel module: importable by everyone, with no database
access. It is adopted below, with gates that keep it pure.

Between `main` and the checkpoint commit, the duplication fixes added 49
`allowedImports` edges and moved two layers (`canonicaljson` 8 to 1,
`interlock` 2 to 5) so the new edges passed the one-way gate. 30 of the edges
came from two consolidations.

## Root causes

1. **`sources` mixed two concerns.** It owns injected nondeterminism: clock,
   virtual clock, id generators, id prefixes, lease default. The timestamp
   consolidation added time-text encoding to it (`FormatTime`, `ParseTime`,
   `Expired`, `LiveDeadline`, `FormatWireTime`). That is a persistence and
   contract representation concern, so every store and every pure domain that
   touches a timestamp had to import `sources` (24 edges, plus `interlock`
   moving above `sources`).
2. **Pure domains handled storage text.** Nine domain packages parsed stored
   timestamp strings and six formatted them. A rule should compare instants;
   the store converts text at the boundary.
3. **Trivial primitives were centralised.** Wrapping stdlib `sha256` as
   `canonicaljson.Sum`/`HasSumLength` forced `canonicaljson` edges into
   low-layer packages (`eventlog/domain`, `evidence/domain`, `evidence/wire`,
   `engine/store`, `replay/store`, `episodeledger/store`), which led to
   moving `canonicaljson` down to layer 1. A one-line stdlib call is not a
   business rule and needs no owner. Seven stores also built `"sha256:"+hex`
   text, a contract rendering that does not belong in a store.

## Target design

| Concern | Owner | Rule |
| --- | --- | --- |
| Where time and ids come from | `sources` | Clock, Virtual, Generator, Prefix*, `OrPhysical`, `OrRandom`, `OrLease`, `DefaultLease`, `NowUTC`, `NowFunc`, `DetachedContext`. No time-text encoding. |
| Durable timestamp text | `kernel` | `kernel.FormatTime` (fixed-width UTC) and `kernel.ParseTime`. A pure package: standard library only, no database, file, network, random source or wall-clock read. Any production package may import it without declaring the edge. Used where a package must encode or decode an instant: SQL columns, digest preimages, wire documents. |
| Instants in rules | domain | Records that cross from store to domain carry `time.Time`. The store parses once and fails closed with a wrapped error. Deadline checks use `!expires.After(now)` directly; no helper. |
| Instants inside digest preimages and wire documents | the document's owner | The app or store passes the formatted text in, or the contract package (`contractsv1`) owns the pinned wire form. A pure domain does not import a codec for this. |
| Domain-separated canonical digests | `canonicaljson` | `Digest`, `DigestSum`, `Seal`, `Verify`, `VerifySum`, `EncodeDigest`, `DecodeDigest`, `ContentDigest`, `VerifyStored`. Only callers that already import it use them. |
| Raw SHA-256 of stored bytes | the caller, stdlib | `crypto/sha256` inline. No wrapper. |
| Rendering a stored 32-byte digest as `sha256:<hex>` | app or domain | Stores return raw bytes; a layer that already imports `canonicaljson` calls `EncodeDigest`. |

## Gate rules for the rework

- `architecture_flow_test.go` returns to `main` (`canonicaljson` 8, `interlock`
  2), plus `storagetest` and `kernel` (layer 0, exempt from the sideways rule).
- `internal/kernel` is a foundation package that every package may import. Its
  gates: `TestKernelStaysPure` (standard library only; no `database/sql`, `os`,
  `os/exec`, file or network packages, `syscall`, or random sources; no
  `time.Now`, `Since`, `Until`, `Sleep` or timers) and `TestKernelHasNoSubpackages`. A symbol is admitted only when two
  or more modules need it and it is a stable representation rule, not
  business behaviour.
- `allowedImports` may gain only: store to table-owning module edges (for
  example store to `episodeledger`), `storage` edges for stores that now use the
  time codec, and edges an issue names. No new `sources` or `canonicaljson`
  edge for time or hashing.
- Each remaining edge added since `main` is listed below with its reason.

## Edges kept (reason)

After the rework, 17 edges differ from `main` (down from 49), and the layer
table differs only by `kernel` and `storagetest`. `canonicaljson` is back at
layer 8 and `interlock` at layer 2.

| Edge | Reason |
| --- | --- |
| `actions/store` to `approvalledger`, `actions/store`, `evidence/store`, `policy/store`, `replay/store` to `episodeledger` | A store reads a table through the module that owns it (DUP-013, DUP-004, DUP-014). This is the direction the architecture wants. |
| `api/transport`, `control`, `control/app`, `device/app`, `evidence/app` to `sources` | They use injected-source vocabulary: `NowUTC`, `NowFunc`, `OrLease`, `DetachedContext` (DUP-023, DUP-024, DUP-027). Not time text. |
| `executor/native/store` to `evidence` | The evidence read budget has one owner (DUP-023). |
| `executor/remote/domain`, `runtime/composition` to `contractsv1` | The risk class and the tenant constant are contract vocabulary (DUP-001, DUP-023). |
| `replay/transport` to `spec` | The dispatch policy constants are owned by `spec` (DUP-005). |
| `runartifact/store` to `actionport` | The unresolved command states are owned by `actionport` (DUP-006). |
| `storagetest` to `storage` and `migrations` | Test support opens migrated temporary databases. |

`canonicaljson.Sum` and `HasSumLength` stay for packages that already import
`canonicaljson`; packages without that edge use `crypto/sha256` directly. The
`sha256:<hex>` text form now lives in `kernel`, and `canonicaljson.EncodeDigest`
and `DecodeDigest` delegate to it.

## Group 3 result

Modules: evidence (with wire), authority, control, notify, runtime, executor
(native, remote, fixture), contractsv1, api, storage/internal/store, testsupport,
cmd/agentic-stream. No `sources.FormatTime/ParseTime/Expired/LiveDeadline/FormatWireTime`
user is left in them. No `allowedImports` or `packageLayers` change.

Files changed:

- cmd/agentic-stream/quarantine_command.go, experiment_e2e_test.go, experiment_live_feed_test.go: `kernel.FormatTime`/`ParseTime`.
- internal/notify/internal/store/{append,audit,poison,retention}.go: `kernel.FormatTime` at the SQL boundary.
- internal/evidence/internal/store/writes.go: `kernel.FormatTime`.
- internal/evidence/internal/wire/{token,fingerprint}.go: token claims and the request fingerprint keep the fixed-width instant text (same bytes, digests unchanged) through `kernel.FormatTime`/`ParseTime`; `CallFingerprint` hashes with `crypto/sha256` instead of `canonicaljson.Sum`.
- internal/evidence/internal/domain/{contracts,reservation}.go: `canonicaljson.Sum`/`HasSumLength` replaced by stdlib `sha256` (gate failure for the evidence domain).
- internal/authority/internal/store/{bindings,claims,events,reconciliation,safety}.go: `kernel.FormatTime`/`ParseTime`; the domain already carried `time.Time`.
- internal/control/internal/store/{owner,epoch}.go and internal/control/internal/app/{owner,epoch}.go: lease and epoch-state store methods now take `time.Time` (`ClaimLease`, `RenewLease`, `ReleaseLease`, `HoldsLease`, `RecordEpochState`, `SupersedeEpoch`) and encode inside the store; the app no longer formats. Tests: control/internal/app/app_test.go, control/internal/store/store_test.go, control/cost_reservation_test.go.
- internal/runtime/internal/store/costs.go, runtime/internal/store/recovery_clock_test.go, runtime/internal/app/admission_test.go: `kernel.FormatTime`.
- internal/storage/internal/store/{storage,storage_test}.go: migration `applied_at` via `kernel.FormatTime`.
- internal/executor/native/internal/domain/evidence_query.go (tool-argument window parse via `kernel.ParseTime`) and native store evidence_tool_test.go.
- internal/contractsv1/internal/domain/cloud_event.go: the CloudEvent envelope digest owns its trimmed RFC 3339 wire form (`CloudEvent.wireTime`); contracts_test.go pins the digest projection with the literal text `2026-08-04T22:26:00Z` instead of calling a helper.

Domain record types changed: none. Store method signatures changed in control
only (above); the control cost-ledger facade (`CostLedger.Reserve/Settle`,
`ApplyCostCeilings`, `controltest.SetCostLimit`) keeps its caller-supplied
`now string`, because its callers live in other modules; the epoch kill path
formats the text it hands to `Settle` with `kernel.FormatTime`.

Goldens regenerated: none. Token claims, fingerprint and CloudEvent digests are
byte-identical (token claim JSON is pinned by `TestTokenEncodingAndIntegrity`,
the envelope time by `TestCloudEventEnvelopeDigestBindsMetadataAndData` in contractsv1).

Edges needed: none. Open item for the gate owner:
`TestTimestampTextHasOneOwner` (architecture_timetext_test.go) still names
`sources` as the owner and rejects `time.RFC3339Nano` in
`contractsv1/internal/domain/cloud_event.go` (the pinned CloudEvent wire form);
it needs to allow the `contractsv1` wire form and name `kernel` as the owner of
the durable layout.

## Group 1 result

Modules: actions, policy, decisions, watch, interlock (approvalledger touches no
time text and is unchanged). No `sources.FormatTime/ParseTime/Expired/LiveDeadline/FormatWireTime`
user is left in these trees; time text uses `kernel.FormatTime/ParseTime` only
where a column, digest preimage or wire document needs text. No gate table edit.

Domain record types changed (text to `time.Time`; the store parses once with
`kernel.ParseTime` and fails closed with a wrapped error):
- watch `domain.Condition.ExpiresAt`
- policy `domain.IntentRecord.ExpiresAt`, `ApprovalRecord.ExpiresAt`, `ApprovalAssertion.ExpiresAt`;
  `Tx.PendingApprovalExpiry` and `Tx.AssertionBinding` return `time.Time`;
  `Tx.ReplaceGovernance` takes `time.Time`
- actions `domain.OutboxLease.Until`, `Lease.Until`, `IntentRow.ExpiresAt`, `ApprovalRow.ExpiresAt`
- decisions: no record change; the domain parses the wire `expires_at`/`valid_until` text
  with `kernel.ParseTime` and compares with `After`.

Files changed:
- interlock: `interlock.go`, `internal/store/store_test.go` (no `sources`/`storage` import; uses `kernel.FormatTime`)
- watch: `internal/domain/condition.go`, `internal/store/watches.go`, tests `internal/domain/condition_test.go`, `internal/store/store_test.go` (new fail-closed test), `internal/app/watch_test.go`
- decisions: `internal/domain/intent.go`, `internal/domain/validator.go`, `facade_test.go`
- policy: domain `records.go`, `workflow.go`, `routing.go`, `commands.go`, `approval_notice.go`, `definition.go`; app `evaluate.go`, `approval_presentation.go`, `principals.go`; store `approval_lifecycle.go`, `approval_lookup.go`, `approval_reads.go`, `command_writes.go`, `evaluations.go`, `intent_reads.go`, `principal_writes.go`; tests `routing_test.go`, `transaction_test.go`, `reads_test.go`, store `fixture_test.go`, app `policy_test.go`, `approval_order_test.go`
- actions: domain `dispatch.go`, `candidate.go`, `authorization.go`; store `authorization.go`, `candidates.go`, `leases.go`, `outcomes.go`, `reconciliation.go`; tests under domain (`candidate_test.go`, `dispatch_test.go`, `authorization_test.go`, `fixture_test.go`), store (`store_test.go` new fail-closed test, `fixture_test.go`), app (`dispatch_test.go`, `service_test.go`)

Goldens regenerated: none. Digest preimages keep the fixed-width text because they
use `kernel.FormatTime` as before (command `created_at`, outcome `observed_at`,
approval notification and assertion `expires_at`).

Behaviour changes on purpose:
- An unreadable stored expiry or lease time now fails the read with a wrapped error
  instead of being treated as expired. Tests that used an unparseable string now use
  the zero `time.Time`, which the rules treat as expired.
- Decision/intent `expires_at` parse errors keep the text `parse time: ...`.

Edges needed: none. The gate failures that remain in `go test -count=1 .` name other
modules (engine, episodeledger, episodes, eventlog, ingress, replay, spec, kernel).

## Group 2 result

Scope: engine, situations, eventlog, ingress, cognition, episodeledger, episodes, spec, replay. The codec is `internal/kernel` (`kernel.FormatTime` / `kernel.ParseTime`, same fixed-width layout as the old `sources.FormatTime`), per the owner's design change. No `sources.FormatTime/ParseTime/Expired/LiveDeadline/FormatWireTime` or `storage.FormatTime/ParseTime` user is left in these modules (grep is empty). No allowedImports or packageLayers edit was made. Layout is unchanged, so no digest, id or golden moved; no golden was regenerated.

Principle applied: rules and records carry `time.Time`; the store parses once at the SQL boundary and fails closed with a wrapped error; stores and use cases that write a timestamp take a `time.Time` and the store formats it. `kernel` text is used only where a package must encode an instant: SQL columns, digest preimages (engine heartbeat timer ids, situations snapshot document, rejection ids, trigger evaluation event id) and document decoding (engine fact times, ingress simulator trace records).

Domain record types changed:
- engine: `Checkpoint.Watermark` is `time.Time` (zero before the first record); `StoredSituation.FirstEventTime/LatestEventTime/UpdatedAt` are `time.Time` (the domain `parseTimes` is gone; `WatermarkFor(previous)` and `TimerWatermark(checkpoint, now)` take instants and `TimerWatermark` no longer returns an error).
- eventlog: `ScannedEvent` carries parsed `EventTime`, `IngestedAt`, `ObservedAt *time.Time`; `StoredTimes`, `DecodedEvent` and `ScannedEvent.Decode` are deleted; the store parses (errors keep `parse event_time` / `parse ingested_at` / `parse observed_at`). Quarantine operations (`QuarantineEnvelope`, `QuarantineRaw`, `ReleaseQuarantine`, `RedriveQuarantine`, `Quarantine`) and `ValidQuarantine/ValidRelease` take `time.Time`.
- episodeledger: `Rejection.At` is `time.Time`, `RejectionID` takes an instant (hashes `kernel.FormatTime`, so ids are unchanged); store and facade methods that took `now`/`startedAt`/`endedAt` text now take `time.Time` (`InsertEpisode`, `InsertAttempt`, `FinishAttempt`, `OwnerHoldsLease`, `Abandon*`, `Conclude`, `Supersede*`, `UpsertSchedulerItem`, `CoalesceSchedulerItems`, `SettleEpisodeCost`, the queue reads).
- replay: `Comparison.CreatedAt` is `time.Time`.
- episodes: runner and store `now` parameters and `DecisionInsert.Now`/`ValidatedIntentInsert.Now` are `time.Time`; `Runner.runtimeNow` is deleted.
- cognition: `CoalesceTriggerWork`, `InsertItem`, `WithdrawSuperseded` take `time.Time`.

Cross-module edges I needed (not in my modules, no gate change): the cost ledger (`control`, `CostLedger.Reserve/Settle`) and `approvalledger.WithdrawSuperseded` still take time text, so episodes and cognition stores call `kernel.FormatTime` at that call; if their owners move to `time.Time` those two lines shrink. `cmd/agentic-stream/quarantine_command.go` was already adapted by its owner to the `time.Time` eventlog quarantine API.

Tests: new `TestCorruptStoredTimesRefuseTheReadWithTheirColumn` and `TestCorruptCheckpointWatermarkRefusesTheRead` (engine store), `TestReadRecordsRefusesCorruptStoredTimesInColumnOrder` (eventlog store) replace the deleted domain parse tests; `TestCoalesceReturnsExactlyTheItemsItFlipped` now uses a distinct coalesce instant.

Still failing at the repo root and naming my modules, but not about time: `internal/engine/internal/store`, `internal/episodeledger/internal/store`, `internal/eventlog/internal/domain` and `internal/replay/internal/store` import `internal/canonicaljson` for `Sum` / `EncodeDigest` (root cause 3, the hashing rework: engine `operator_state.go` and `situation_reads.go`, episodeledger `episode_reads.go`, eventlog domain `quarantine.go` and `event.go`, replay store `store.go` and `recorded_ledger.go`). I left these for the hashing group because they are a separate concern. `TestTimestampTextHasOneOwner` also still fails (it names `internal/kernel` and `contractsv1`, not my modules) because it still points at `sources`.
