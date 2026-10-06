# Sources ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Source | An injected provider of something that makes a run non-reproducible: time or identifiers. Rules receive sources as parameters and never read the wall clock or a random generator themselves. | `Clock`, `Generator` | — |
| Clock | The runtime's only source of time: `Now` and `NewTimer`. | `Clock` | — |
| Physical clock | Wall time in UTC, for live processing. | `Physical` | quality `physical` |
| Virtual clock | Deterministic time that moves only when advanced, for replay and tests. | `Virtual`, `NewVirtual` | quality `virtual` |
| Advance | Moves a virtual clock forward and fires every due timer in due-time order, ties broken by scheduling order. | `Virtual.Advance` | — |
| Clock quality | Whether time is virtual or physical, recorded with replay evidence. | `Quality` | `physical`, `virtual` |
| Generator | Produces unique identifiers from a prefix; safe for concurrent use. | `Generator.New` | `<prefix><suffix>` |
| Random generator | Base64url random identifiers, for live runs. | `Random` | `evt_AbC…` |
| Deterministic generator | Numbered identifiers, global per instance, for tests and replay. | `Deterministic` | `evt_1`, `evt_2` |

| Prefix | The identity space of an identifier, one per kind of durable record: `evt_`, `sit_`, `epi_`, `att_`, `sch_`, `trg_`, `dec_`, `int_`, `apr_`, `cmd_`, `out_`, `pol_`, `ver_`, `lease_`, `rec_`, `aud_`, `shd_`. Pure vocabulary; deterministic code may use it, but never `Random` or `Physical`. | `Prefix*` | identifier prefix |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| `time.Now` in rules | A `Clock` or a time parameter | Wall-clock reads break deterministic replay; the architecture gate forbids them in domain layers. |
| Package `clock`, package `ids` | `sources` | Both were tiny leaf packages for the same purpose; merged. |
| `ids.Prefix*` | `sources.Prefix*` | Same constants, now beside the generators. |
| Sequence | Deterministic generator | `Sequence` was test-only and was deleted earlier. |
