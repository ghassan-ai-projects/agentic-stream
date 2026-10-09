# api

Status: done
Round: 15

Audited in round 15 with [runtime](runtime.md). The module is JSON/HTTP plus
Server-Sent Events; its tests now prove status codes, error bodies, SSE framing
and client disconnect through `httptest`, with no fixed port and no sleep.
Production code is unchanged.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/api` (facade) | 100.0% | 100.0% | 1.4 s | 1.3 s | 2 / 0 | 2 / 0 |
| `internal/api/internal/domain` | 95.1% | 100.0% | 1.2 s | 1.2 s | 7 / 0 | 10 / 49 |
| `internal/api/internal/transport` | 85.0% | 96.8% | 1.8 s | 2.4 s | 16 / 29 | 24 / 70 |

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 10 findings before
(all in `transport`), 0 after. Repository `golangci-lint`: 0 issues. `dupl` at
threshold 60: 0. `time.Sleep`, `context.Background()`, `t.Skip` in tests: none.
`go test -race -shuffle=on -count=3`: passes.

Mutation checks (each reverted): the GET-only rule of the stream, the
repeated-event suppression, the allow-list and per-event authorization, the
constant-time control token comparison and the approval bearer check; every one
fails named tests.

## Findings and changes

### Removed
- `events_test.go` (health endpoints) and the health half of `boundaries_test.go`
  and `http_test.go`: three files proved the same liveness/readiness contract
  from three angles; one table (`TestHealthEndpoints`) replaces them (T2).
- `serveUntilCanceled` and `cancelOnFlushWriter` (cancel after two flushes):
  they proved "the handler stops when the context ends" by counting flushes and
  waited on a one-second `time.After`; replaced by a real `httptest` server whose
  client reads frames and disconnects (T5).
- The tautology import keeper `_ = errors.New` in the domain tests.

### Renamed or moved
- `http_test.go` → `control_test.go` (it proved the epoch control endpoints),
  absorbing the control cases of `boundaries_test.go`.
- `events_test.go` + `boundaries_test.go` health cases → `health_test.go`.
- `sse_cursor_test.go` → `sse_delivery_test.go` (the file proves
  `sse_delivery.go`); its test is now `TestAFilteredPageStillAdvancesTheResumeCursor`.
- domain `rules_test.go` → `stream_test.go` (cursor, timing, page size, dedup,
  allow-list, frames, problems) and `approval_test.go` (bearer matching, approval
  input, config), next to the source each proves.
- New `runtime_handler_test.go` (route table), `sse_fixture_test.go` (events,
  `httptest` session helper).

### Improved
- T6: every test and subtest is parallel (10 findings to 0). The approval
  table test was in `package transport` without `t.Parallel()`; it is an
  external test now.
- T4: `t.Fatal(rec.Code, calls, rec.Body.String())` without got/want became
  messages that state the expectation; problem bodies are decoded and checked
  (`type`, `code`, `status`, content type) instead of substring matches.
- T5: SSE tests wait on frames read from the stream, never on time; the only
  clock is the handler's own poll and idle intervals (1 ms in tests) and a 10 s
  failure timeout on the client request.
- T9: one SSE fixture family (`appendSSEEvents`, `connect`, `nextFrame`,
  `disconnect`); events are appended in one transaction so lag is deterministic.

### Added
Domain (95.1% to 100%): `TestParseCursor` (overflow, boundaries and the exact
message), `TestStreamTimingReplacesNonPositiveIntervalsWithDefaults`,
`TestFramesFollowTheServerSentEventsFormat` (unencodable data is refused),
`TestApprovalConfigIsConfiguredOnlyWithEveryCallbackAndCredential` (five
absent-part cases), bearer cases for a blank expected token and a prefix-only
token.

Transport (85.0% to 96.8%):
- `TestSSEStreamsDurableEventsInFramesAndResumesAfterTheCursor`: five cases
  (from the start, `Last-Event-ID`, `cursor` query, header beats query, past the
  end), response headers and the exact frame shape.
- `TestSSEDeliversEventsAppendedAfterTheSubscriberConnected`: the poll path
  (was 0%), custom `retry`.
- `TestSSEKeepsAnIdleConnectionAliveWithComments`: the keep-alive path (0%).
- `TestSSEStopsWhenTheClientDisconnects`: client disconnect ends the handler.
- `TestSSEShowsEachTenantOnlyItsOwnEvents` and
  `TestSSEResolvesTheTenantFromTheRequestWhenTheDeploymentProvidesOne`.
- `TestSSEDeliversOnlyEventTypesTheSubscriberMayRead`: allow-list and per-event
  authorization, skipped events leave the cursor correct.
- `TestSSEEndsWithAStreamErrorWhenAFollowUpReadFails`: storage failure and
  `subscriber_too_slow` after connect end with a `stream_error` control frame
  that carries no `id` and then close the stream. The lag case repeats its
  append (`provokeUntilStreamError`, at most 50 rounds) while a poll that
  straddled the commit delivered the events instead of seeing the lag.
- `TestSSEStopsWhenTheConnectionCannotBeWrittenTo` (write fails on the connected
  comment, on the first event) and `TestSSERefusesAWriterThatCannotStream`.
- `TestSSEAnswersARefusedResumeWithAProblemBeforeAnyStream` (expired cursor with
  its audit row; too slow).
- `TestRuntimeHandlerMountsOnlyTheSurfacesItIsGiven` (metrics, events, health).
- Approvals: success bodies for present and resolve, relay binding that ignores a
  `relay` query parameter, `Allow` header on 405, unparseable decision, and
  `Configured()` fail-closed for each missing part.
- Control: configuration precedes authentication, no control routes without a
  control.

### Speed
Not slow (all packages about 1 to 2 s, mostly race start-up). The SSE tests run
in parallel with each other; none waits on a timer.

## Production code touched
- none.

## Invariants proven here
The module is a transport edge; it proves no invariant alone but guards the
entry to three:
- Invariant 7 (policy revalidates every Intent immediately before dispatch): a
  human approval reaches policy only through the authenticated, relay-bound
  edge, and nothing is called before authentication and input validation pass:
  `TestApprovalsAreRejectedBeforeAnyCallbackRuns`,
  `TestApprovalsPresentAndResolveBoundToTheRegisteredRelay`,
  `TestApprovalsFailClosedUntilEveryPartIsConfigured`. Signature, single use and
  re-evaluation are proven in policy.
- Operator control (drain and kill; refusing later work is proven in control and
  runtime) is an authenticated operator action:
  `TestControlEndpointsRequireExactToken`,
  `TestControlAuthenticationPrecedesMethodAndStateChanges`.
- Invariant 10 (decisions are explainable): admitted and refused work is
  delivered as durable, tenant-scoped, cursor-resumable notifications:
  `TestSSEStreamsDurableEventsInFramesAndResumesAfterTheCursor`,
  `TestSSEShowsEachTenantOnlyItsOwnEvents`,
  `TestSSEAnswersARefusedResumeWithAProblemBeforeAnyStream`.

## Open items
- Flake fixed after review (orchestrator saw one failure under load; reproduced
  as 113 failures in 720 runs with 12 copies of the test binary running at once,
  `-race -shuffle=on`). Cause: the same `notify` read race as the item below, in
  its second form. `ReadPage` checks the lag from `TenantBounds` and then reads
  the rows in a separate autocommit query. A poll that read the highwater before
  the test's append committed and the rows after it saw lag 0, delivered the two
  new events (`id: 2`) and moved the cursor, so the lag never appeared. The
  test now repeats the append until the lag is observed atomically; after the
  change 0 failures in 720 runs of the SSE tests and in 360 runs of the whole
  package under the same load. Production is unchanged; the fix belongs in
  `notify` (read bounds and rows in one snapshot), which would also remove the
  extra rounds.
- Bug in `notify` (not changed here; a finding for its owner), first seen as a flake of the live-delivery test:
  `store.Tx.TenantBounds` reads the tenant's oldest cursor and its highwater as
  two separate autocommit queries. When a subscriber reads an empty tenant stream
  while the first event is appended between the two reads, `HasOldest` is false
  and `HasNextCursor` is true, `RefuseResume` sees `cursor 0 < NextCursor-1` and
  answers `cursor_expired` (the client is told to resnapshot, an audit row is
  written). It showed as a one-in-many flake of a test that connected to an empty
  stream and then appended; the test now seeds one event first. Fix: read both in
  one snapshot (one query, or one read transaction). Needs a regression test in
  `notify/internal/store`.
- `classify` still has no test for `ErrNotificationPoison` (503 `notification_retry`)
  inside a stream; the mapping itself is covered in the domain table. Producing
  a poison notification needs the notify module's poison fixture.
- The `EventFrame` and `ControlFrame` marshal-error branches in the transport
  (`writeEvent`, `writeControl`) cannot be reached with real notifications (they
  are always encodable); left uncovered rather than faked.
