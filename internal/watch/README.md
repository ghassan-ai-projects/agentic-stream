# Watch module

Watch owns derived-trigger watches: bounded, expiring conditions that an
approved command installs through the effect port, that matching evidence fires
at most `max_fires` times, and that expire without losing their audit rows. It
never reaches dispatch, policy or reasoning.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `New(Config)`, the effect-port methods `Dispatch` and `DispatchAuthorized`, `Expire`, `FireEvent` |
| App | Install, fire, fire-event and expire use cases; the SQLite-busy retry; logging a skipped expression |
| Domain | Payload validation, watch identity, idempotency comparison, CEL validation and evaluation, scope matching |
| Store | Opaque transactions, watch SQL, and runtime-owner and interlock checks on the mutating transaction |

`New` requires the database, runtime ownership check and interlock; a missing
one is a constructor error, never a silently skipped check. Composition without
a runtime owner passes an explicit always-pass check, as for policy and actions.
The clock defaults to the physical clock.

Install validates the route, then the payload (identity before expression before
expiry), then in one transaction accepts an identical earlier install of the same
watch identity, rejects a conflicting one, asserts ownership and the interlock,
inserts, and reads the row back. A fire asserts the same guards, expires due
watches, requires the watch's Situation and target to match, evaluates the
expression (an evaluation error is a logged no-fire), records the fire once per
event and spends one allowance, disabling the watch at zero.

[Migration record](../../docs/watch-reference-module-2026-10-06/README.md).
Architecture gates enforce a pure domain, opaque transactions, exact write
ownership of `watch_conditions` and `watch_fires`, and facade delegation.
