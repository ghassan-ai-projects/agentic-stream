# Decisions vocabulary

This file is the canonical vocabulary for `internal/decisions`. The package
checks worker proposals against the trusted episode context and the compiled
intent authority. It does not persist Decisions or authorize effects.

| Term | Meaning | Code name | Contract or storage name |
| --- | --- | --- | --- |
| Decision | A worker's proposed outcome for one dispatched episode attempt | `Result` after `Validate` | `decision_id`, `raw_json`, `decision_sha256` |
| Intent | A typed action proposal nested in a Decision; policy later revalidates it | `Intent` | `intent_id`, `intent_json`, `intent_sha256` |
| Attempt binding | Episode attempt identity and fencing value trusted by the runtime | `Input.EpisodeID`, `Input.AttemptID`, `Input.Fence` | `episode_id`, `attempt_id`, `fence` |
| Snapshot binding | Immutable Situation identity/version and digest attached to the episode | `Input.SituationID`, `Input.SituationVersion`, `Input.SnapshotDigest` | `situation_id`, `situation_version`, `snapshot_digest` |
| Episode authority | Intent types and maximum risk allowed for this attempt | `Input.AllowedIntentTypes`, `Input.RiskCeiling` | request `allowed_intent_types`, `risk_ceiling` |
| Compiled intent authority | Validated per-type risk, parameter schema, presets, writable fields and policy limits | `IntentCatalog` | digest-bound `intent_catalog` |
| Trusted validation time | Runtime-supplied instant used for Decision and Intent freshness checks | `Input.Now` | `valid_until`, `expires_at` |
| Canonical Decision | RFC 8785 JSON whose domain-separated digest is checked before acceptance | `parseDecision` | `canonical_json`, `decision_sha256` |
| Validation refusal | Typed, fail-closed reason and field details for a rejected proposal | `ValidationError` | `reason`, `details` in validation evidence |

## Retired words

| Retired word or shape | Replacement | Reason |
| --- | --- | --- |
| Public mutable `IntentEntry` / `IntentCatalog.Entries` | Opaque compiled `IntentCatalog` | Compiled risk, schema and preset authority must not be editable by consumers. |
| `Result.Document` and `Intent.Document` as returned evidence | The caller's original Decision bytes and the validated Intent's canonical bytes | The maps were unused by production consumers and invited mutation or mistaken digest use. |
| Decisions package as a persistence owner | Episode store owns Decision records; policy store owns governed Intents | The validator selects no table and executes no SQL. |

The episode store retains the original worker Decision bytes for audit and uses
the verified Decision digest. The validated Intent retains its canonical bytes
and digest for the episode-to-policy handoff.
