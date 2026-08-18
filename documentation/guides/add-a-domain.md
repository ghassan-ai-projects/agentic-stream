# Add a domain

The runtime is intended to support domains through data and `SituationSpec`,
not through new branches in the stream engine.

## 1. Define the event vocabulary

Add or update event schema data in:

```text
internal/eventschema/registry_data.json
```

Include event type, schema version, payload fields, units, and constraints that
the registry can validate. Extend registry tests and deliberately update the
golden digest only when the data change is reviewed.

## 2. Define simulator input only when needed

For simulator traces, update:

```text
internal/ingress/simulator_data.json
```

Keep adapter channel-to-field mapping out of Go literals. Add conversion tests
for the new edge cases.

## 3. Author the SituationSpec

Declare inputs, partition/entity identity, event-time policy, windows,
operators, Situation reducers/phases, cognitive triggers/budgets, and allowed
Intent catalog. Start from the predictive-maintenance example and validate:

```bash
agentic-stream validate path/to/domain.situation.yaml
```

## 4. Define governed Intents

Every Intent has a type, risk class, parameter schema, policy mode, and expiry
behavior. If the domain uses the shared aquaculture fixture catalog, update
[`internal/episodes/testdata/aquaculture_intents.json`](../../internal/episodes/testdata/aquaculture_intents.json)
and preserve the cross-repo digest parity test. Do not put catalog data in Go.

## 5. Add deterministic traces and tests

Cover duplicate event identity and conflicting reuse, out-of-order and
allowed-late events, source-health loss, Situation correction and
reconsideration deduplication, episode cancellation/stale output, invalid
Decision/Intent and policy denial, idempotent effect/unknown outcome, and
replay hash/no-effect behavior.

## 6. Update the public docs

Add the domain to the event catalog, status/evidence map, guide if it is a
supported example, and limitations if any behavior is only fixture/test
support. Run `make docs-check` and the repository test suite.

## Next reads

- [First SituationSpec](../getting-started/first-situation.md)
- [Event catalog](../reference/event-catalog.md)
- [Quality gates](../governance/quality.md)
