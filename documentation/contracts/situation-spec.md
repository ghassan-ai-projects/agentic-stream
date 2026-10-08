# SituationSpec v1

A `SituationSpec` defines how the runtime follows a domain condition. You
write it in YAML or JSON. The compiler checks its structure and rules, converts
it to canonical JSON, and computes a `sha256:` digest before deployment.

## Runtime authority

The runtime embeds and validates against
[`internal/spec/internal/domain/schema.json`](../../internal/spec/internal/domain/schema.json), the only
copy of the schema; the compiler tests are the authority for semantic behavior
beyond it.

## Shape

The required top-level sections are:

```yaml
apiVersion: agentic-stream/v1
kind: SituationSpec
metadata: {name: example, version: 1.0.0}
inputs: []
time: {}
windows: []
operators: []
situation: {}
cognition: {}
actions: {}
```

Start from the working example:
[`predictive-maintenance.situation.yaml`](../../docs/design/examples/predictive-maintenance.situation.yaml).

## Compilation

Compilation performs:

1. YAML parsing with duplicate-key protection;
2. JSON Schema validation;
3. semantic name, reference, operator, CEL, unit, and budget validation;
4. deterministic normalization;
5. canonical JSON serialization and digest computation;
6. compiled runtime structures for stream and cognition packages.

Validate with:

```bash
agentic-stream validate path/to/spec.yaml
agentic-stream validate --json path/to/spec.yaml
```

The digest identifies the compiled content. An episode also records digests
for its prompt, objective, and catalogs so the runtime can check exactly which
definitions it used.

## Authoring constraints

- Names and versions follow the schema patterns.
- Inputs declare event type, schema version, payload schema, partition key,
  entity type, classification, and payload limits.
- Time policy declares bounded out-of-orderness, idle timeout, allowed
  lateness, and the late-data policy.
- Operators may only consume declared inputs/windows and produce declared
  outputs.
- CEL is restricted to deterministic functions and bounded expressions.
- Cognition declares triggers, thresholds, lanes, executor identity, and hard
  budgets.
- Actions declare the allowed Intent vocabulary, risk, parameter schema, and
  policy mode: `automatic` or `approval` (the default). `automatic` never
  relaxes the risk route: R2 always needs approval and R3/R4 are denied. An
  Intent that must never run is not declared at all; undeclared types are
  refused by Decision validation.

## Next reads

- [First SituationSpec](../getting-started/first-situation.md)
- [Stream processing](../design/stream-processing.md)
- [Decision and Intent](decision-intent.md)
