# Native executor reference module (2026-10-06)

`internal/executor/native` is the in-process episode executor. It is an adapter
module with no tables: a facade of type aliases and one-statement constructors
over `internal/app` (the bounded model/tool loop: budgets, tool runs, repair,
artifacts), pure `internal/domain` (provider and tool contracts, budget
accounting, request decoding, Decision validation, evidence-scope rules, the
deterministic provider and in-memory artifact store), `internal/transport` (the
OpenAI-compatible HTTP provider and its JSON and SSE decoding) and
`internal/store` (the scope-bound event-log evidence tool). Vocabulary:
[UBIQUITOUS_LANGUAGE.md](../../internal/executor/native/UBIQUITOUS_LANGUAGE.md).

Layers: domain 10, transport 11, store 11, app 12, facade 13. The public names
(`New`, `Config`, `Executor`, provider, tool and artifact types, evidence tool,
batch runner) are unchanged; they are aliases of the layer types.

## Behavior that must not change

Budget enforcement in the loop (model calls, tokens, cost, tool calls, tool
result bytes, provider retries), failure reason strings, repair limited to one
attempt, the evidence tool only narrowing its row and byte bounds and never its
entity, provider errors classified as interrupt, cancellation or retryable.

## Deliberate behavior changes

None. The evidence argument rules moved from the tool to
`domain.EvidenceScope.Query` with identical error messages.

## Held for the owner

`RunBatch`/`RunBatchJSON` and `MemoryArtifactStore` are test-only in production
terms (see the dead-code review). They were moved with the layers, not removed.

## Rating

Layering 8, domain rules 8, fail-closed safety 8, ubiquitous language 8, tests 8,
data-level encapsulation 7, type safety 7, simplicity 7. Weaknesses in order: the
facade is mostly aliases, so the layer types are public by another name; the
loop keeps mutable counters on one struct instead of a pure state value that the
domain advances; `store` still builds JSON rows (encoding belongs to domain);
budget vocabulary duplicates `executor/remote`.
