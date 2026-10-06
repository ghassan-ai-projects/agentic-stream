# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, design, plan | Review | Complete |
| 1 | `canonicaljson`: move mechanism into `internal/domain`, facade, tests per layer, language file, gates, layer table | Cross-repo vectors and parity pass unchanged; layer, purity and delegation gates; injected failures | |
| 2 | `ids`: domain, random adapter, facade, language file, gates | Existing id tests; determinism gates; injected failures | |
| 3 | Docs, status and follow-ups | Full CI | |

## Behavior that must not change

Canonical bytes and digests for every vector (shared and native-only), the
rejection of every malformed input, `sha256:` reference formats, identifier
prefixes and shapes (`prefix` + 16 hex digits deterministically, 12 random bytes
base64url otherwise), and the panic on an entropy failure.

## Deliberate changes

None to behavior. Layer table entries move up one level.

## Deferred

A prefix-only package that the deterministic packages may import (follow-up 9).
