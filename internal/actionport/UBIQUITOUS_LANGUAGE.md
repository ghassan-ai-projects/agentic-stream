# Actionport ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Command | A validated, policy-approved input to an effector. It carries its own idempotency key and policy digest. | `Command` | `commands` (owned by `actions`) |
| Effector | The only interface that crosses from the action plane into an external system. It must honor the command's idempotency key. | `Effector.Dispatch` | — |
| Effect | The provider's response to a dispatched command: provider result, observed effect, and whether verification is pending. | `Effect` | `outcomes` (owned by `actions`) |
| Verification pending | The provider acknowledged receipt only; independent feedback must establish physical success. | `Effect.VerificationPending` | `verifications` status `awaiting` |
| Authorization | The final runtime check, run immediately before a concrete effector accepts the effect. | `Authorization.Check` | — |
| Authorized effector | An effector that takes an `Authorization`, closing the gap between validation and acceptance. | `AuthorizedEffector.DispatchAuthorized` | — |
| Device state verifier | Verifies a device-backed command with one fresh state query. | `DeviceStateVerifier.VerifyDeviceCommand` | — |
| Unknown outcome | The request may have reached the provider. The dispatcher records that reconciliation is required and never retries blindly. | `UnknownOutcomeError`, `IsUnknownOutcome` | command status `outcome_unknown` |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Failure (for an unknown outcome) | Unknown outcome | A failure may be retried; an unknown outcome must be reconciled. |
| Adapter (for the port) | Effector | The port is the effector; adapters are the implementations in `device` and elsewhere. |
