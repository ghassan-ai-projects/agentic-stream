# Decisions language

This dated planning copy records the vocabulary used to plan the migration.
The canonical glossary is
[`internal/decisions/UBIQUITOUS_LANGUAGE.md`](../../internal/decisions/UBIQUITOUS_LANGUAGE.md).

| Term | Meaning | Code name | Contract or storage name |
| --- | --- | --- | --- |
| Decision | Worker-proposed outcome bound to one episode attempt | `Result` | `decision_id`, `raw_json`, `decision_sha256` |
| Intent | Typed proposed action that policy revalidates | `Intent` | `intent_id`, `intent_json`, `intent_sha256` |
| Attempt binding | Runtime-trusted episode, attempt and fence | `Input` identity fields | `episode_id`, `attempt_id`, `fence` |
| Snapshot binding | Immutable Situation identity, version and digest | `Input` snapshot fields | `situation_id`, `situation_version`, `snapshot_digest` |
| Episode authority | This attempt's permitted Intent types and maximum risk | `AllowedIntentTypes`, `RiskCeiling` | request authority fields |
| Compiled intent authority | Per-type risk, schema, presets, writable fields and limits | `IntentCatalog` | digest-bound `intent_catalog` |
| Trusted validation time | Injected time for freshness checks | `Input.Now` | `valid_until`, `expires_at` |
| Validation refusal | Fail-closed typed reason and field details | `ValidationError` | validation evidence fields |

| Retired word or shape | Replacement | Reason |
| --- | --- | --- |
| Mutable `IntentCatalog.Entries` and public `IntentEntry` | Opaque compiled catalog | Prevent caller mutation of authority. |
| Returned Decision/Intent document maps not consumed in production | Original Decision bytes and canonical validated Intent bytes | Avoid duplicate, mutable evidence that can be mistaken for persisted input. |
| Decisions as a storage owner | Episode and policy stores retain their declared ownership | The validator performs no database work. |
