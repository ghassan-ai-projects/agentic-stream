# Replay language

Names mean the same thing in conversation, code, storage and audit trails.

## Terms

| Term | Meaning | Code name | Storage / wire name |
| --- | --- | --- | --- |
| Replay session | One isolated run of a trace against a compiled spec | `app.Session` | — |
| Isolated database | Fresh database created for one session, never reused | `transport.OpenIsolatedDatabase` | `<trace>.replay.db` |
| Trace | Ordered JSONL evidence file replayed as the only source | `TracePath` | `event_log` rows via ingress |
| Epoch | Virtual clock start derived from the earliest valid trace time | `domain.EpochFromEarliest`, `Virtual` clock | — |
| Worklist | Executable episodes a replay produced, keyed by episode key | `domain.ReplayEpisode`, store `EpisodeWorklist` | `scheduler_items` + `episodes` join |
| Episode key | Stable `situation/version/trigger` identity of a worklist row | `domain.EpisodeKey` | ledger `episode_key` |
| Recorded ledger | Durable, read-only set of worker results for a past run | `domain.RecordedLedger` port | caller-supplied |
| Recorded entry | One verified worker result bound to an episode key | `domain.RecordedEntry` | decision JSON + digests |
| Shadow trial | Paired effect-free execution of baseline and Tamoz on one snapshot | `app` shadow use case | `shadow_comparisons` row |
| Baseline | Deterministic non-model executor of a trial | `domain.BaselinePolicy`, `BaselineExecutor` port | `executor_version` `deterministic-baseline-v1` |
| Tamoz | The model-under-test executor of a shadow trial | `ShadowExecutor` port | comparison `tamoz` fields |
| Shadow comparison | Sealed report of one shadow trial | `domain.Comparison` | `shadow_comparisons` record |
| Finding | Deterministic, non-effectful replay observation | `domain.Finding` | result JSON |
| Capability | Explicit non-credential adapter a mode requires | `domain.Capabilities` | — |
| Mode | Effect-safe replay mode (deterministic, recorded, shadow) | `domain.Mode` | result `mode` |
| Versions hash | Ordered SHA-256 over all situation-version digests | `domain.HashVersionDigests` | result `versions_hash` |
| Admission window | Earliest admission time and expiry of a scheduler item | `domain.AdmissionWindow` | `created_at`, `not_before`, `expires_at` |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| `replayItem` | `domain.ReplayEpisode` | One word for the worklist identity; the public projection and the private row no longer split the vocabulary |
| `caps` | `capabilities` | No abbreviations in domain prose |
| `executableSchedulerItems` | admission candidates / `EpisodeWorklist` | Names the domain decision, not the storage scan |
| `applyCapabilities` closure threading | capability phase | The `after` callback escapes layering; the app names the phase |
