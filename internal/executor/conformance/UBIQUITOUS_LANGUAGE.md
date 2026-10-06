# Executor conformance ubiquitous language

| Term | Meaning | Code name |
| --- | --- | --- |
| Conformance | The semantic contract every episode executor must meet, shared by the in-process fixture, the native executor and the streamed worker. | `Run` |
| Fixture request | The smallest valid request: one situation, one intent type, a risk ceiling and a budget. | `FixtureRequest` |
| Produced outcome | An outcome whose Decision is bound to the request's attempt, fence, snapshot and prompt digests. | `checkProducedOutcome` |

This package is test support. No production code imports it, so `deadcode`
reports it as unreachable by design.
