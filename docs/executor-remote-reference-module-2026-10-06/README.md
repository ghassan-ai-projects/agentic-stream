# Remote executor reference module (2026-10-06)

`internal/executor/remote` adapts the streamed `EpisodeWorker` protocol to the
episode `Executor` port. It is an adapter module with no tables: a facade over
`internal/app` (bound the attempt by wall time, build and capability-bind the
wire request, negotiate, consume the stream), pure `internal/domain` (wire
request mapping, handshake and evidence-config checks, event-by-event stream
validation, trusted budget accounting) and `internal/transport` (the gRPC calls
and cancellation-to-context mapping). Vocabulary:
[UBIQUITOUS_LANGUAGE.md](../../internal/executor/remote/UBIQUITOUS_LANGUAGE.md).

Layers: domain 10, transport 11, app 12, facade 13. The domain works on
protocol values (`runtimev1` messages) as plain data; the facade keeps
`NewExecutor`, `NewExecutorWithEvidence`, `Execute`, `CapabilityFactory` and
`AttemptCapabilityIssuer`. The layer table was renumbered: every package that
was at layer 11 or above moved up by three (runtime transport 14, runtime app
15, composition 16, replay 14/15/16, runtime 17, cmd 18).

## Behavior that must not change

Handshake and stream error texts and precedence, budget accounting (maximum of
event-derived and worker-reported totals), fencing and sequence checks, decision
digest verification, evidence capability issued per attempt, RPC cancellation
matching `context.Canceled` and `context.DeadlineExceeded`.

## Deliberate behavior changes

- A nil `*Executor` no longer answers "worker client is not configured"; it
  panics. Constructors never return nil, and an executor without a client still
  refuses to run.

## Tests

Integration tests against a real in-process worker moved with the use case to
`internal/app`; wire-request and stream rules are tested in `internal/domain`;
`internal/transport` tests call failures and cancellation mapping; the facade
keeps a configuration test. Gates: layer table, allowed imports, repository map,
facade delegation (`architecture_remote_executor_test.go`), and the existing rule
that only executors import the worker protocol and gRPC.

## Rating

Layering 8, domain rules 8, fail-closed safety 9, ubiquitous language 8, tests 8,
data-level encapsulation 8, type safety 7, simplicity 8. Weaknesses in order: the
domain depends on generated protobuf types, so it is pure but not
protocol-independent; the capability issuer lives in `app` with its scope check
inline; `executor/native` and `executor/remote` still duplicate the budget
vocabulary.
