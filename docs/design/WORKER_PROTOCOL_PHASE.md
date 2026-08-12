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

Explicitly deferred from this phase:

- durable cross-restart evidence-call audit/idempotency and process-epoch
  invalidation;
- runtime composition that issues a fresh capability from each persisted
  attempt and removes raw token injection from the executor test seam;
- child-process supervision, remote workers, mTLS, Python fixtures, and
  provider-specific evidence backends.
- stale-attempt lease expiry and process-crash reissue; the existing fence
  model rejects stale output, while recovery policy is a subsequent phase.
- runtime-owned hard accounting for model calls, tokens, tool calls, result
  bytes, retries, and cost. Worker `BudgetUpdated` telemetry is not an
  authorization source.

Those surfaces are subsequent phases. Until they are implemented and tested,
the completion bar's worker conformance gate remains open.

The next phase closes durable audit and runtime-issued token composition. It
still does not add Python workers, child-process supervision, remote transport,
mTLS, or any legacy/N-1 compatibility behavior.
