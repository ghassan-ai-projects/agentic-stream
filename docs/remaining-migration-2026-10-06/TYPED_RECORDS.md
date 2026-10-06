# Typed records rounds

Goal: replace `map[string]any` documents with typed records parsed once at the
boundary, with a closed field set, keeping the original bytes wherever a digest
depends on them. One commit per round, each with focused tests, lint and the
architecture gates. Order is by untyped-map count and by how self-contained the
document is. Rules for every round: no digest, identity or wire change; error
precedence unchanged unless listed; the original document is kept when a digest
is computed over its exact form.

| Round | Target | Maps before | Status |
| --- | --- | --- | --- |
| 1 | `ingress` simulator event (`simulator_event.go`) | 16 | Done: 16 to 13. The record enters as a JSON map and the envelope `Data` is an open contract map, so those stay; the identity, timestamps, value and unit are now one typed `simulatorEvent` parsed once with the same check order and messages. Little more is available here. |

## How far typing can go (563 untyped maps in production code)

Round 1 showed the limit: a map is only worth removing when it is a fixed-shape
record. Most maps are one of three intrinsic kinds, which should stay open:

| Kind | Why it stays a map | Where |
| --- | --- | --- |
| Digest-bound stored documents | The exact bytes feed a digest, so the original document is kept and only read through typed accessors | `authority/evidence.go`, `episodes/reconsideration.go` (nested prior decision, command, outcome evidence), `runartifact/verify.go` |
| CEL inputs | CEL evaluates against `map[string]any` variables (`features`, `delta`, `facts`) | `situations/*`, `cognition/delta.go`, `watch` |
| Untrusted model output | Intent parameters are validated against a per-intent JSON Schema, so the shape is data | `decisions/*` (58), `episodes/intent_catalog.go` |

Realistic candidates (fixed-shape records), ranked by value:

1. **Decision document** built by `executor/fixture`, `replay/baseline.go` and
   the native deterministic provider (about 45 maps): one typed `Decision` with
   canonical encoding, parsed once where it re-enters through `decisions`.
2. **Reconsideration top level** (`episodes/reconsideration.go`, 24): typed
   top-level, outcome and correction records; the nested stored JSON stays open.
3. **Device wire records** (`device/wire/codec.go` and `records.go`, 15): strict
   typed frames with the exact bytes kept for digests.
4. **Simulator and ingress** (done in round 1, little left).

Estimated reachable reduction: about 150 to 200 maps (a third), not all 563.
