# 5. Turn effect routes into data

## Problem

The rule is "domains are data, via the SituationSpec". Effect routing breaks it:
the composition root hard-codes which action routes go to which effector, and
one device adapter has its own field in the runtime config.

## Evidence

- `internal/runtime/effect_routing.go:57-58`: `case "install_watch_condition"`
  and `case "set_indicator", "select_thermal_mode"` are written into the code.
  The same thermal routes appear again at line 109 for verification.
- `internal/runtime/pipeline.go`: `PipelineConfig.SerialEffector
  *device.SerialEffector` is a concrete device type in the generic runtime
  config.
- `CompositeEffector` has exactly three slots: watch, serial, and fallback. A
  second device family needs a new field, a new setter, new `switch` cases,
  and new CLI wiring in `cmd/agentic-stream/effect_profile.go`.
- If no serial effector is configured, verification returns no status and no
  error, so the command looks unverified instead of failing (see
  [item 1](01-fail-closed-safety-dependencies.md)).

## Why it matters

The next domain (for example aquaculture actuators) will have to change
`internal/runtime`. That is the change the domain-as-data rule is meant to
prevent. It also makes it hard to see which routes a deployment supports.

## Recommendation

1. Declare the route-to-effector-family binding as data. It can live in the
   spec's `actions` section or in the device capability catalog, which already
   lists device routes. Example:

   ```yaml
   actions:
     routes:
       install_watch_condition: { effector: watch }
       select_thermal_mode:     { effector: device.serial, verify: true }
   ```

2. Replace `CompositeEffector` fields with a route table,
   `map[route]actionport.Effector`. The composition root builds the table from
   the declared bindings and the effectors that were opened.
3. Startup fails if a declared route has no effector, or if a route is marked
   `verify: true` but its effector does not implement `VerifiedEffector`.
4. Replace `PipelineConfig.SerialEffector` with a list of named effectors,
   `map[string]actionport.Effector`.

This changes a protocol surface (spec or catalog). Write an ADR before
implementing it.

## Done when

- `internal/runtime` contains no route names.
- Adding a device family means adding an adapter in `device`, catalog data, and
  one registration line in the composition root.
- A test proves that startup rejects an unbound route and a verify route whose
  effector cannot verify.
