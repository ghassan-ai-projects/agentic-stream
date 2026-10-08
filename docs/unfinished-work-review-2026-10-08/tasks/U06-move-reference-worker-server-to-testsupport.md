# U06 — Move the reference worker server to test support

Status: done · Decision: **move** · Priority: P2 · Size: M

## Finding

35 `internal/worker` functions are test-only: the validating `Server`
(`Handshake`, `Execute`, stream guard), the request, handshake and stream
validators (`ValidateRequest`, `ValidateHandshake`, `RequireFeatures`,
`NewHandshakeResponse`, `StreamValidator`, `StartedEvent`, `WireError`,
`Limits.Resolved`), and two dialers (`DialEpisodeWorkerSocket` without TLS,
`DialEvidenceSocket`).

The runtime is the gRPC *client* of a worker. Production workers live outside
this module (Tamoz in Ruby; any Go worker in its own process), and because the
package is under `internal/` no external worker can import this server. The
remote executor has its own client-side validators in
`internal/executor/remote/internal/domain`.

## Decision and reasoning

Move the server and its validators to `internal/testsupport/workerfake`. It is a
good fake worker for the remote-executor and evidence tests; it is not
production code. Keeping it in `worker` makes `worker` look like a runtime
component with a large untested-in-production surface.

`worker` keeps what production uses: protocol constants, `ValidateBudget`,
`ValidateEvidenceSocketPath`, `ListenEvidenceSocket`,
`DialEpisodeWorkerSocketTLS`.

- `DialEvidenceSocket` (worker side of the evidence socket) moves with the fake.
- `DialEpisodeWorkerSocket` is deleted; tests call
  `DialEpisodeWorkerSocketTLS(ctx, path, nil)`, which is the production function.

## Done when

- `deadcode ./...` lists none of the 35 under `internal/worker`;
  `internal/testsupport/workerfake` is covered by the U12 allow-list.
- The architecture gates accept the new package (testsupport is importable only
  from tests).
- `documentation/guides/build-a-go-worker.md` points to the fake as an example
  of the contract, not as a library to import.
