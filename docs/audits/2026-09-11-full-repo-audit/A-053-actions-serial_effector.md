# A-053 · `internal/actions/serial_effector.go`

LOC: 178 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The verified output parameter is selected deterministically from catalog data, not from Go map iteration order (`serial_effector.go:125-137`).
- Every exported method in the file has a production caller (`serial_effector.go:58-83`).

## Findings
- **MED F1. `expectedOutput` selects the verified value by map iteration order** — `internal/actions/serial_effector.go:125-137`. The function returns the first `float64` parameter encountered while ranging over the wire command's `parameters` map, skipping only the hardcoded name `"lease_ms"`. Go map iteration order is unspecified, so the moment any route's preset carries a second numeric parameter, the verification verdict (`:118-122`) becomes nondeterministic run-to-run — a command could reconcile `failed` on one attempt and `succeeded` on the next. It also embeds a per-route assumption ("lease_ms is never the output") in Go code that a data-only catalog change can silently invalidate, against the domains-are-data rule. Fix: name the verified output parameter in the catalog (e.g. an `output_parameter` field on `OperationSpec` in `serial_materialize.go`) and read it explicitly; reject at `validate()` time if it is absent or non-numeric.
- **MED F2. `SafeStop` is production-dead** — `internal/actions/serial_effector.go:58-83`. `SerialEffector` is wired only into `CompositeEffector` routing (`internal/runtime/pipeline.go:121`, `composite_effector.go:93-97`), which uses `Dispatch`, `DispatchAuthorized`, and `VerifyDeviceCommand`. Nothing in production calls `SafeStop`, so the governed priority lane exists only behind tests. Either wire an explicit e-stop entry point (CLI/runtime) or remove the method until one exists; leaving it unwired means the catalog's safe stops are unreachable in a running system.

## Checked, not an issue
- P1: errors wrapped `%w`; nil-receiver guards consistent; no shared mutable state (session owns all locking).
- P2: dispatch binds the catalog digest to the handshake-accepted digest before materializing (`:143-149`) and materialization itself re-enforces closed-catalog bounds; `DispatchAuthorized` runs the interlock check immediately before materialization and delivery (`:46-54`); post-send ambiguity always becomes `UnknownOutcomeError` (`:158-163`); rejected commands (pre-send receipt) yield ordinary errors, not unknowns (`:173-176`); verification evidence is digest-bound and boot-checked (`:92-105`).
- P3: `SafeStop`'s evidence-lane branching (`:64-79`) is intricate but each branch is reachable and documented; no duplication with the session files beyond the charged items in A-040.
- P4: effector is a thin adapter; all device semantics live in the session/catalog; no per-domain branches (the fan/LED comment at `:115-117` documents a current two-route invariant, not a branch).
- P5: exported symbols documented; context first param throughout.
- P6: `serial_effector_test.go` covers verification mismatch, boot rollover, rejection, unknown-outcome lanes, receipt caching, and catalog mismatch.
- P7: verification compares exact expected values materialized from the closed catalog (see F1 for the latent ordering hazard).
