# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, design, plan | Review | Complete |
| 1 | `canonicaljson`: move mechanism into `internal/domain`, facade, tests per layer, language file, gates, layer table | Cross-repo vectors and parity pass unchanged; layer, purity and delegation gates; injected failures | Complete |
| 2 | Docs, status and follow-ups | Full CI | Complete |

## Behavior that must not change

Canonical bytes and digests for every vector (shared and native-only), the
rejection of every malformed input, and the `sha256:` reference formats.

## Deliberate changes

None to behavior. Layer table entries move up one level.

## Deferred

A prefix-only package that the deterministic packages may import (follow-up 9).

## Result

- Layers: domain 0, facade 1; 30-odd importers moved up one level by the longest-path check.
- Cross-repository vectors, native-only vectors and the Ruby parity digests pass unchanged; the strict-validation
  and number tests moved with the domain.
- Gates: `architecture_canonicaljson_test.go` (the facade only delegates) plus the generic purity, layering, foundation,
  language and map gates; proven by six injected violations (logic in the facade, a clock read, an `os` import, a foundation
  importing a plane, the domain importing its facade, a missing language file).

## Rating (out of 10)

| Area | Score |
| --- | --- |
| Layering | 8 |
| Domain rules | 9 |
| Fail-closed safety | 9 |
| Ubiquitous language | 8 |
| Tests | 9 |
| Encapsulation | 9 |
| Type safety | 8 |
| Simplicity | 7 |

Weakest: the facade still re-lists seventeen `Domain*` constants by hand, and `Digest`'s parameter is a
`Domain` while the package is also named `domain`. Raising it: generate the constant aliases from the domain
file, or let `Domain` live only in the facade.
