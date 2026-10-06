# IDs ubiquitous language

| Term | Meaning | Code name | Identifier shape |
| --- | --- | --- | --- |
| Generator | Produces unique identifiers from a prefix; safe for concurrent use. | `Generator.New` | `<prefix><suffix>` |
| Random generator | Base64url random identifiers, for live runs. | `Random` | `evt_AbC…` |
| Deterministic generator | Numbered identifiers, global per instance, for tests and replay. | `Deterministic` | `evt_1`, `evt_2` |
| Prefix | The identity space of an identifier. One prefix per kind of durable record. | `Prefix*` | `evt_`, `sit_`, `epi_`, `att_`, `sch_`, `trg_`, `dec_`, `int_`, `apr_`, `cmd_`, `out_`, `pol_`, `ver_`, `lease_`, `rec_`, `aud_`, `shd_` |

Packages that must stay deterministic (the replay domain, the native executor,
the ledger domains) do not import this package because it also holds the random
generator; they keep literal prefixes.

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Sequence | Deterministic generator | `Sequence` was test-only and was deleted. |
| `PrefixArtifact`, `PrefixReplay` | — | Unused; deleted. |
| `pol_` for audit identifiers | `PrefixAudit` | Notification audits are not policy evaluations. |
