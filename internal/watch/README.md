# Watch module

Watch owns derived-trigger watches: bounded, expiring conditions that an
approved command installs through the effect port, that fire when their expression
turns true, at most `max_fires` times, and that expire without losing their audit rows. It
never reaches dispatch, policy or reasoning itself: `FireEvent` reports the
Situation of each fire, and the runtime wakes that Situation in the engine.

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
expression against the features of the watch's own Situation, read from its
runtime state through a `FeatureSource` (the spec's feature names, as of the
latest applied event; a Situation the source does not hold is a logged no-fire;
an evaluation error counts as false and is logged), and fires on a rising edge
only: when the expression holds now and did not at the watch's last evaluation.
The last evaluation (`last_matched`) is seeded at install from the Situation's
features when the service has a `FeatureSource` (`Config.Features`), so a watch
whose expression already holds at install fires only after it has been false;
without a seed the first true evaluation fires. A fire is recorded once per
event and spends one allowance, disabling the watch at zero.

Migration record.
Architecture gates enforce a pure domain, opaque transactions, exact write
ownership of `watch_conditions` and `watch_fires`, and facade delegation.
