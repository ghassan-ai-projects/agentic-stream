# A-027 · `internal/actions/serial_session_state.go`

LOC: 310 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported method in the file has a production caller.
- The device-bind state machine exists once: `bindHandshakeState` (`serial_session.go:181-215`) and `applyRefreshedState`/`bindRefreshedState` (`serial_session_state.go:230-275`) do not duplicate identity/digest/barrier scaffolding with divergent semantics.
- The immutable capability catalog digest is not recomputed per state query (`serial_session_state.go:101`).

## Findings
- **MED F1. Production-dead accessors, and the only barrier-clearing path has no production caller** — `internal/actions/serial_session_state.go:19-21,29-31,42-49,53-60,85-88,281-310`. `DeviceID`, `AuthorityEpoch`, and `SafeState` have zero callers repo-wide (not even tests). `ReconciliationRequired`, `RefreshState`, and `ResolveReconciliation` are test-only. Critically, `ResolveReconciliation` is the sole caller of `storage.ReconciliationStore.Resolve` (`internal/storage/authority.go:515`), and `BindState` keeps `status='required'` for the same boot on session reopen (`internal/storage/authority.go:412-414`) — so in production there is no path that ever clears a durable reconciliation barrier for the current boot: after any untrusted exchange, every ordinary serial command fails with `ErrReconciliationRequired` indefinitely. Either wire a production reconciliation flow (dispatcher-driven: reconcile the command ledger, then resolve the device barrier with `QueryStateEvidence`) or remove the dead symbols and make the gap explicit in the design docs.
- **MED F2. Duplicated, already-divergent bind-state machine** — `internal/actions/serial_session_state.go:230-275` vs `internal/actions/serial_session.go:181-215`. Both paths apply device identity, firmware/capability digests, safe state, state digest, assert authority, call `reconciliation.BindState`, and transition `reconciliationRequired` with telemetry. They already diverge: the handshake sets `stateQueryRequired = priorBarrier || required` (`serial_session.go:203`) while the refresh unconditionally clears it (`:270`); the refresh fails the session closed on `BindState` error (`:265-267`) while the handshake leaves cleanup to `OpenDeviceSession`'s defer (`serial_session.go:96-100`). Some divergence is intentional, but the shared scaffolding invites drift in safety-critical transitions. Extract one bind helper parameterized by the handshake/refresh differences.
- **LOW F3. Catalog digest recomputed per query** — `internal/actions/serial_session_state.go:101` (and `serial_effector.go:143`). `CapabilityCatalog.Digest()` re-validates and re-canonicalizes the whole catalog on every `QueryState` and every dispatch although the catalog is immutable after `LoadCapabilityCatalog`. Compute it once at load/handshake and reuse.

## Checked, not an issue
- P1: all session state is guarded by `s.mu` (accessors `:73-80`, transitions under lock); `requireReconciliation` persists with `context.WithoutCancel` + bounded timeout (`:205-206`) so a canceled exchange cannot skip the durable barrier; errors wrapped `%w`.
- P2: boot change invalidates cached receipts before acceptance (`:232-236`); evidence carries typed digests and is bound to the latest state digest before resolution (`:296-298`); identity change fails the session (`:113-116`).
- P3: `readString` helper keeps accessor boilerplate minimal; no speculative abstraction beyond F1/F2.
- P4: storage boundary respected; no transport logic here beyond the typed `DeviceTransport`.
- P5: exported symbols documented; `errors.Join` used for compound failures.
- P6: `serial_session_test.go` covers refresh fencing, boot invalidation, and failed-refresh invalidation.
- P7: stable identities (device/boot/epoch) bound at handshake and re-checked on every refresh.
