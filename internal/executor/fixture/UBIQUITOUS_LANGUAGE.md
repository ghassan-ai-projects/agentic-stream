# Fixture executor ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Fixture executor | A deterministic episode executor for explicit demo and replay fixtures. It calls no model and owns no lifecycle or effects. | `Executor`, `New` | — |
| Outcome | The typed result of one attempt: a Decision and its reasons, returned to the episode runner for authoritative validation. | `episodes.Outcome` | `episode_attempts` (owned by `episodeledger`) |
| Fixture decision | A digest-bound Decision that proposes an R1 maintenance-ticket intent for the snapshot's phase and trigger. | `fakeIntent` | `decisions` |

Production composition refuses this executor outside an explicit fixture mode;
the runtime's admission step enforces that.
