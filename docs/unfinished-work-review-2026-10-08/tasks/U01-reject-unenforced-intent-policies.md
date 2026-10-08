# U01 — Reject unenforced intent policies

Status: done · Decision: **fix (reject at compile)** · Priority: P0 · Size: S

## Finding

The spec schema accepts `actions.intents[].policy` in
`automatic | approval | deny | simulate`
(`internal/spec/internal/domain/schema.json:678`, same in the design contract).
Only `approval` has an effect:

- `internal/episodes/internal/domain/intent_catalog.go:166` turns the value into
  `{"requires_approval": policy == "approval"}`; `deny` and `simulate` become
  `false`.
- `internal/policy/internal/domain/routing.go` (`RiskRoute`) routes R0/R1 to
  `automatic` when `requires_approval` is false.

So an R0 or R1 intent that the author declared `deny` is **dispatched
automatically**, and `simulate` behaves like `automatic`. This fails open, which
breaks invariant 7 in spirit: the author's governance setting is ignored without
an error. `documentation/overview/limitations.md` already admits the four values
are "not separately enforced"; it does not say that two of them fail open.

No spec or data file in the repository uses `deny` or `simulate`
(`aquaculture_intents.json` included), so removing them breaks nothing.

## Decision and reasoning

Remove `deny` and `simulate` from the schema enum, so compiling a spec that
uses them fails with a clear error.

- `deny` is equivalent to not declaring the intent: the Decision validator
  already rejects intent types that are not in the catalog. A second way to say
  "not allowed" adds a code path and a test matrix for nothing.
- `simulate` has no implementation. The live device "emulator" profile already
  covers rehearsing effects, and counterfactual replay is being deleted
  ([U02](U02-delete-counterfactual-replay.md)).
- Enforcing `deny` in policy instead would change the catalog document, its
  digest and the policy route table to support a value nobody uses.

`automatic` on R2 and above is already safe: R2 goes through calibration or
approval and R3/R4 are denied whatever the policy says. Document that `automatic`
never relaxes the risk route.

## Steps

1. Write the failing test first: compile a spec with an R0 intent declared
   `policy: deny`, run it to policy evaluation, and assert that nothing is
   dispatched. It must fail on the current code, which proves the defect.
2. Remove `deny` and `simulate` from both schemas
   (`internal/spec/internal/domain/schema.json`,
   `docs/design/contracts/situation-spec-v1.schema.json`).
3. Turn the test into a compile-rejection test: the spec fails `validate` with a
   message naming the field.
4. Add a policy test pinning that `policy: automatic` on R2/R3/R4 never
   dispatches without calibration or approval.

## Done when

- Both tests pass; `make ci-check` is green.
- `documentation/contracts/situation-spec.md` and
  `documentation/overview/limitations.md` list only `automatic | approval`, and
  the limitations section on policy values is removed.
- `release-status.json` drops "complete Intent policy-mode enforcement" from
  `partial`.
