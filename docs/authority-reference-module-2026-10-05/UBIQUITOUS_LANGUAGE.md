# Ubiquitous language: device authority

The device-authority context answers one question: may this runtime send an
ordinary command to this physical target right now? It also keeps the durable
evidence of every answer. The words below are used the same way in
conversation, documentation, Go identifiers, audit events and table columns.
A word that is not here should not appear in the module's public API.

## Identities

| Term | Meaning | Code | Storage |
| --- | --- | --- | --- |
| **Owner** | The runtime process that holds the singleton runtime lease. Identified by owner epoch and owner instance. | `Owner{Epoch, Instance}` | `owner_epoch`, `owner_instance` |
| **Owner epoch** | Opaque generation ID of one ownership period. A new owner always has a new epoch. | `Owner.Epoch`; `OwnerEpoch` in `device` configuration | `owner_epoch` |
| **Owner instance** | Identity of the process that holds the epoch. | `Owner.Instance` | `owner_instance` |
| **Device boot** | One power-on of one device. A reboot is a new device boot. | `DeviceBoot{DeviceID, BootID}` | `device_id`, `boot_id` |
| **Target** | One addressable output of a device, for example `fan-01`. | `string` | `target` |
| **Device-wide subject** | An audit subject about the whole device, not one output. Its target is the device ID. | `DeviceSubject(device, owner)` | `target = device_id` |

## Admission

| Term | Meaning | Code |
| --- | --- | --- |
| **Ordinary admission** | The owner's lease is valid and its epoch is neither draining nor killed. Every ordinary operation passes it as the first step of its own transaction. | `admitOrdinary`, `withAdmittedTx` |
| **Priority path** | Writes that must succeed whatever the authority state: safe-stop stages, opening a reconciliation after authority loss, releasing a claim, and recording safety evidence. They make the device safer, give up authority, or preserve evidence, and they skip ordinary admission. | `withPriorityTx` |
| **Authority loss** | The owner's lease expired, or its epoch is draining or killed. | `ErrRuntimeOwnerBusy`, `ErrEpochDraining`, `ErrEpochKilled` (from `control`) |

## Target claims

| Term | Meaning | Code | Storage / event |
| --- | --- | --- | --- |
| **Target claim** | An owner's leased right to command one target on one device boot. | `TargetClaim{Target, Device, Owner}` | `device_target_claims` |
| **Held claim** | The claim currently recorded for a target, with its fence, lease and status. | `domain.HeldClaim` | one row per target |
| **Live** | A held claim that is active and whose lease has not expired. | `HeldClaim.Live(now)` | `status = 'active' AND lease_until > now` |
| **Claim lease** | How long a claim stays live without renewal. | `Config.ClaimLease` | `lease_until` |
| **Claim fence** | Counter that increases on every takeover. A live renewal by the same owner keeps it, so a stale owner can never present a current fence. | `HeldClaim.Fence` | `claim_fence` |
| **Acquire** | Write a claim on a target that has no live claim by another owner. | `ClaimAcquired` | `claim_acquired` |
| **Renew** | The same owner claims again while its claim is still active. | `ClaimRenewed` | `claim_renewed` |
| **Reject** | Another owner holds a live claim. The attempt is audited and refused. | `ClaimRejected`, `ErrTargetClaimBusy` | `claim_rejected` |
| **Release** | The exact holder gives up its active claim. | `ReleaseClaim` | `claim_released` |
| **Not owned** | The caller is not the exact live holder. | `ErrTargetClaimNotOwned` | — |

## Commands

| Term | Meaning | Code | Storage |
| --- | --- | --- | --- |
| **Command binding** | Immutable record tying a command to the target, device boot and owner that delivered it, plus its digest. Written immediately before delivery. Repeating the identical binding is idempotent; any difference is a conflict. | `CommandBinding`, `BindCommand` | `device_command_bindings` |
| **Command evidence** | Reconciliation evidence presented for one bound command. It must name the bound target and the bound device boot. | `VerifyCommandEvidence` | — |
| **Unresolved command** | A command bound to a device boot whose outcome the action ledger still treats as unknown, reconciling or under manual review. | `CountUnresolvedCommands` (store) | `commands.status` |

## Reconciliation

| Term | Meaning | Code | Storage / event |
| --- | --- | --- | --- |
| **Device state** | The typed state document a device reports on handshake or query. Canonical JSON plus its SHA-256 digest. | `DeviceState`, `RecordDeviceState` | `state_json`, `state_sha256` |
| **First seen** | When the device's first state was recorded. | — | `first_seen_at` |
| **Reboot** | A device state whose boot ID differs from the recorded one. | `StateRebooted` | — |
| **Reconciliation** | A case opened for a device boot when the runtime can no longer trust what the device did: a reboot, an untrusted receipt, or an unknown outcome. | `domain.Reconciliation` | `device_reconciliation` |
| **Reconciliation required** | A reconciliation is open. Ordinary commands to the device are blocked. This is the **barrier**. | `ReconciliationRequired`, `ErrReconciliationRequired` | `status = 'required'` |
| **Clear** | No reconciliation is open. | `ReconciliationClear` | `status = 'clear'` |
| **Open** a reconciliation | Move the device boot to required and audit the reason. | `OpenReconciliation`, `OpenReconciliationAfterAuthorityLoss` | `reconciliation_opened` |
| **Reconciliation evidence** | Independent, digest-bound feedback about one device boot: source, type `device_state_feedback`, typed state, feedback, and three SHA-256 references. | `ValidateReconciliationEvidence` | `resolution_evidence_json` |
| **Resolve** | Record evidence and an outcome for an open reconciliation. | `ResolveReconciliation` | `reconciliation_recorded` |
| **Resolution outcome** | `succeeded` or `failed` clears the reconciliation; `manual_review` keeps it required. | `ResolutionOutcome` | `last_resolution_status` |
| **No open reconciliation** | Resolution was attempted when the device boot is clear. | `ErrNoOpenReconciliation` | — |

## Safety

| Term | Meaning | Code | Storage / event |
| --- | --- | --- | --- |
| **Safe stop** | Priority command that brings a device to its safe state. | — | — |
| **Safe-stop stage** | `requested`, `completed` or `failed`. Recorded on the priority path. | `SafeStopStage` | `safe_stop_*` events |
| **Safe-stop latch** | Once any stage is recorded for a device boot, that boot stays latched. There is no clear operation; only a new boot resets it. | `SafeStopLatched` | — |
| **Safety event** | Evidence for the soak verdict: a zero-tolerance violation or a physical transition. | `SafetyEvent`, `RecordSafetyEvent` | `device_safety_events` |
| **Complete physical evidence** | A physical transition marked complete that names its source and a SHA-256 evidence digest. | `PhysicalEvidenceComplete` | — |

## Audit

| Term | Meaning | Code | Storage |
| --- | --- | --- | --- |
| **Authority event** | Append-only audit record of a claim, reconciliation or safe-stop transition, with canonical details and their digest. | `domain.AuthorityEvent` | `device_authority_events` |
| **Subject** | The target, device boot and owner an authority event is about. | `AuthorityEvent.Subject` | `target`, `device_id`, `boot_id`, `owner_*` |

## Retired words

| Do not say | Say instead | Why |
| --- | --- | --- |
| authority epoch | owner epoch | One concept, one name, matching the columns. |
| device lifetime | device boot | "Boot" is concrete and matches `boot_id`. |
| bind state, bound state | record device state | "Bind" is reserved for command bindings. |
| barrier open | reconciliation required | An open barrier sounds like a passable one. |
| `Require` / `Required` | `OpenReconciliation` / `ReconciliationRequired` | A verb and a predicate that differ by one letter. |
| `Assert` (claim) | `AssertClaim` | Distinguishes it from `AssertRuntime`. |
| opening boot | — | Always equal to the current boot; the column is dropped. |
| opened at (for the first state) | first seen at | The column never recorded when a reconciliation opened. |
| `TargetAuthority`, `ReconciliationStore`, `SafetyLedger` | `authority.Service` | One entry point for one context. |
