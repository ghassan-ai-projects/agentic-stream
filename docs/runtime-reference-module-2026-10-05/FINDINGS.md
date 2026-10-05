# Findings

- `pipeline.go` combines public configuration, concrete plane construction,
  maintenance goroutines and ownership transactions. `pipeline_run.go` mixes
  pipeline steps, ingress adapter creation, event-log paging and policy SQL
  transaction plumbing.
- `service.go` owns readiness/heartbeat orchestration while `recovery.go` opens
  ownership/recovery units of work. RecoveryCoordinator is externally used only
  by tests; keep it internal to the use case rather than preserve an unused facade.
- `worker_runtime.go` combines executor selection, environment credentials,
  evidence listeners, gRPC serving, connection negotiation and teardown.
  `worker_runtime_config.go` mixes pure flag consistency with certificate I/O.
- `effect_routing.go` contains pure route classification beside effect calls.
- Runtime mutates no domain tables directly. Cost configuration, recovery,
  policy and owner fencing already call the table-owning modules. Preserve their
  transaction boundaries; do not invent repositories or new lifecycle owners.
- Optional controls are existing explicit simulation/test composition modes.
  Preserve their semantics in this structural migration. Live CLI composition
  continues supplying real owner/epoch checks.
- Existing private tests prove watch pagination, recovery claim timestamps and
  executor selection. Move them with the responsible layers; retain public
  integration tests and add facade/layer-boundary coverage.
