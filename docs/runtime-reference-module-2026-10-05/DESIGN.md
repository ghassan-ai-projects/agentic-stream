# Design

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade | Public configuration/report aliases and delegation | Orchestrate batches, own connections or open transactions |
| Composition | Construct lower planes and adapt configuration to app dependencies | Write lifecycle rows or run business use cases |
| App | Ordered batches, approval use cases, readiness/heartbeat and worker setup sequencing | Import SQL/storage/network or open files |
| Domain | Batch/recovery reports, worker option validation, routing and heartbeat interval rules | Read clocks or perform I/O |
| Store | Original transaction joins, policy operations, fenced cost application, owner/recovery plumbing | Own another module's lifecycle semantics |
| Transport | Ingress files/live UDS, native/remote executor resources, TLS/certificate and evidence gRPC plumbing | Decide permission or mix transport into app orchestration |

Public operations remain NewPipeline/run/start/close and approval operations,
NewService/start/ready/close, and NewWorkerRuntime/executor/errors/close.
Composite effect routing is internal to app and composition. Test-only public
CompositeEffector and RecoveryCoordinator are removed; the readiness use case still performs atomic
recovery. No schema, protocol, key, digest or permission changes are planned.

Natural dependency levels will be recorded explicitly as layers are added;
existing import allowlists and reasoning/replay isolation remain enforced.
No lint, file-size or coverage thresholds may be weakened.
