# Worker protocol phase status

This phase freezes and proves the current-version Go worker boundary. It is a
deliberately narrow vertical slice, not a claim that the full worker release
gate is complete.

Implemented:

- pinned `protoc`, Go protobuf/gRPC generators, committed Go stubs, and CI drift
  checking;
- exact current protocol and contract version handshake, with no legacy or
  N/N-1 behavior;
- immutable request validation, W3C trace-context validation, signed-fence
  identity checks, event sequence checks, terminal enforcement, bounded event
  count/bytes, and panic sanitization;
- a streamed `EpisodeWorker` adapter to the existing fenced episode executor;
- wall-time budget propagation and cancellation persistence using a detached
  bounded database context.

Implemented in the live read-boundary slice:

- HMAC-SHA256 capability tokens with issuer, audience, attempt, fence, tenant,
  situation version, entity, tool, time-range, row/byte, trace, and expiry
  claims; verification is fail-closed and short-lived by default;
- strict v1 `evidence.get` arguments, typed bounded query results, per-call
  deadline cancellation, result-byte/row enforcement, duplicate call rejection,
  and no access to effectors, credentials, filesystem, shell, or arbitrary
  network;
- runtime-owned Unix sockets with absolute-path validation, private directory
  checks, mode `0600`, Unix-only dialing, and ownership-safe cleanup;
- actual EvidenceTools-over-UDS integration coverage.
- a separate-process Go worker conformance fixture using a real private Unix
  socket and the same shared semantic checks as the in-process fixture.
- SQLite evidence-call reservation and completed-result ledger with composite
  identity, request digests, concurrent reservation serialization, integrity
  checks, and current-attempt fencing.
- `AttemptCapabilityIssuer` and an evidence-aware worker executor constructor
  that issue a fresh scoped token per dispatch, binding the attempt identity
  and trace from the trusted request. Tools, evidence range, limits, expiry,
  and runtime epoch are composition inputs; the application composition root
  still owns deriving and wiring them from durable runtime state.

Explicitly deferred from this phase:

- automatic startup recovery/reclamation and process-epoch invalidation for
  unfinished calls; the current ledger exposes explicit reclaim operations but
  does not wire them into startup yet;
- automatic startup epoch generation, recovery, and reissue of unfinished
  attempts; the issuer currently requires the runtime composition to provide
  its authoritative epoch and evidence range.
- production child-process supervision, remote workers, mTLS, and non-Go fixtures, and
  provider-specific evidence backends.
- stale-attempt lease expiry and process-crash reissue; the existing fence
  model rejects stale output, while recovery policy is a subsequent phase.
- runtime-owned hard accounting for model calls, tokens, tool calls, result
  bytes, retries, and cost. Worker `BudgetUpdated` telemetry is not an
  authorization source.

Those surfaces are subsequent phases. The current worker conformance gate is
closed for the Go protocol fixture; production supervision and deployment
rehearsals remain operational hardening.

The next phase closes automatic startup epoch generation, recovery, and fenced
reissue of unfinished attempts. It still does not add non-Go workers,
child-process supervision, remote transport, mTLS, or any legacy/N-1
compatibility behavior.
