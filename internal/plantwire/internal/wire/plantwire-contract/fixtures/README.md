# Plant wire fixtures

| Folder | Contents | A conformant side |
|---|---|---|
| `valid/` | One record per type, one kind file (`KIND.json`) and one board mapping (`BOARD.json`) | Accepts each |
| `invalid/` | `{schema, fails_at, record}`: a record that breaks one rule, and the member where it breaks | Refuses each (`malformed`), at that member |
| `rules/` | `{schema, keyword, fails_at, document}`: a board mapping that passes its schema and breaks one rule of SPEC section 15 | Refuses each with that keyword, at that member |
| `must-ignore/` | `{schema, record}` with members no schema knows | Accepts each and ignores the extra members |
| `injection/` | `{schema, expect, record}` with hostile strings in descriptions and string values | Accepts each as data, quotes the strings when showing them to a model, takes no action from them |
| `exchanges/` | JSON Lines: a header `{exchange, summary, gap?, host_environment?}`, then steps; `seq` runs from 0 by 1 per direction unless the header declares `gap`; `host_environment` (`simulated`, `hardware` or `mixed`, default `simulated`) is the environment the host under test is set to (SPEC section 2) | Reproduces the expected reactions |

An exchange step is one of:

- `{"from": "node" | "host", "record": {...}}`: a record one side sends;
- `{"expect": {"from": ..., "type": ..., "fields": {...}}}`: the other side must next send a record of that type whose
  members include those fields; the type `verdict` is the host's own conclusion about a command (`verified: false` means the
  host must not count it as done), not a record;
- `{"wait_heartbeats": n}`: the host stays silent for `n` heartbeat intervals;
- `{"now_ns": t}`: the node's time (world time for a simulated node) is now `t`.

Every command fixture's `idempotency_key` is the digest of its content (SPEC §9), except where an exchange reuses a key on
purpose; `NBIRTH` carries the real `contract_digest` (this folder's `DIGEST`), `capability_digest` and each kind's `kind_digest` (SPEC §5); `valid/KIND.json` is the first kind of `valid/NBIRTH.json` without its `kind_digest`. A kind file never carries a
`kind_digest` (`invalid/KIND-has-kind-digest.json`, even a correct one): the digest is computed from the file, and a stale
member must not be accepted and ignored. A kind file's metric ranges also need `min` below `max`; a schema cannot say that,
so a conformant side checks it as well.

The `environment-*` exchanges cover SPEC section 2: which node a host of each environment accepts, and the node-versus-boards
invariants (a `simulated` node has only simulated boards, a `hardware` node only hardware boards, a `mixed` node at least one
of each), which a schema cannot say and a host checks at the handshake; a contradiction is refused even by a `mixed` host.
`invalid/NBIRTH-board-no-environment.json` and `invalid/NBIRTH-board-unknown-environment.json` cover the schema side: every
board names its environment, and a board is `simulated` or `hardware`, never `mixed`.

The `lying-result-*` exchanges cover one command per class (setpoint, start, safe stop): the device reports success while the
observations do not move. `verified-result` is the positive control: the observation moves, so the host counts it done.

`valid/BOARD.json` maps the pump kind of `valid/KIND.json` to a pump controller: a scaled argument and reading (`freq_dhz`,
scale 5, in both directions), a fixed raw argument, a copied reading and a raw enumerated word translated by a `table`. The
`BOARD-*` fixtures in `invalid/` break the schema; those in `rules/` break a rule a schema cannot say (member names, raw words,
an argument set twice, uneven table rows, scales that disagree), so a conformant side checks those as well. Every expectation
names only a `device`, `command_id` and `boot_id` some record of its exchange carries.
