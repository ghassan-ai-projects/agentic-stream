# 1. Make safety dependencies required (fail closed)

## Problem

Safety checks are added after construction through `With*` setters. If a setter
is never called, the check is skipped, and nothing reports it. There are 21
`With*` setters in production code. Several of them carry release-blocking
guarantees: interlock, epoch control, and runtime ownership.

## Evidence

| Location | What happens when the setter is not called |
| --- | --- |
| `internal/policy/policy_command.go:88` | `interlock == nil`: the interlock check is skipped |
| `internal/policy/policy_evaluate.go:36` | `epochControl == nil`: the policy epoch check is skipped |
| `internal/policy/policy_store.go:16` | `owner == nil`: the ownership check is skipped |
| `internal/actions/dispatcher.go:117` | `interlock == nil`: dispatch calls plain `Dispatch`, so the **final authorization path is skipped entirely** |
| `internal/actions/dispatch_authorization.go:132` | `interlock == nil`: the command interlock check is skipped |
| `internal/episodes/runner_quarantine.go:96` | `epochControl == nil`: "no refusal" |
| `internal/engine/engine_apply.go:183`, `internal/runtime/pipeline.go:187` | No owner: no fence |
| `internal/runtime/effect_routing.go:109` | No serial effector: verification returns `"", nil, nil` (unverified, no error) |
| `internal/runtime/pipeline_composition.go:33-41` | A nil `Executor` falls back to `FakeExecutor`, and a nil `Effector` falls back to `SimulatedEffector` |

`runtime.composePolicy` and `composeDispatcher` wire these dependencies today,
so the live path is correct. The risk is in every other path: a new composition,
a new test harness, or a refactor that forgets one line. Any of these silently
turns off a safety gate, and none of the architecture tests would fail.

## Recommendation

1. Move safety dependencies into constructors as required parameters, for
   example `policy.NewGateway(policy.Config{Interlock: …, Epoch: …, Fence: …})`.
   The constructor returns an error when a parameter is nil.
2. For tests and replay, add explicit types that say "disabled", such as
   `interlock.Disabled{}` and `control.Unfenced()`. A reader can see the choice
   at the call site, and `grep` can find every place that uses it.
3. Remove the `nil` fallbacks for `Executor` and `Effector` in
   `pipelineExecutionDefaults`. Production must pass them explicitly. Tests use
   the test kit from [item 8](08-shared-test-kit.md).
4. Keep `With*` only for real observability options (`WithTelemetry`,
   `WithLogger`).
5. Add an architecture test that forbids `if x.interlock == nil { return nil }`
   style guards in `policy`, `actions`, `episodes`, `engine`, and `watch`.

## Done when

- No safety gate can be skipped by leaving out an optional setter.
- Every place that disables a gate uses a named "disabled" type.
- Existing tests pass. New tests show that each constructor rejects a missing
  safety dependency.

Related: [item 2](02-ownership-fence-primitive.md) introduces the `Fence` type
that these constructors take.
