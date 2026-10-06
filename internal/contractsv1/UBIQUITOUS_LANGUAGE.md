# Contracts v1 ubiquitous language

| Term | Meaning | Code name | Wire name |
| --- | --- | --- | --- |
| Envelope | The normalized event inside the runtime: identity, entity, event time, quality flags and data. | `Envelope`, `ValidateEnvelope` | CloudEvent JSON |
| CloudEvent | The external JSON form of an envelope. | `CloudEvent` | `specversion`, `tenantid`, `partitionkey` |
| Entity reference | The domain entity an event belongs to (type and id). | `EntityRef` | `entity` |
| Classification | The sensitivity of an event's data, in rising order. | `Classification` | `public`, `internal`, `confidential`, `restricted` |
| Quality flag | A coded note that evidence is degraded (late, unit-converted, and so on). | `QualityFlag` | `quality` |
| Payload hash | SHA-256 of the original normalized payload. | `PayloadHash` | `payload_sha256` (event quarantine) |
| Trace context | W3C `traceparent` and `tracestate`, kept durably so asynchronous work can link back. | `TraceContext`, `ParseTraceContext` | `traceparent`, `tracestate` |
| Schema | An embedded JSON Schema for one contract document. | `SchemaName`, `Validate`, `SchemaID` | `schemas/v1/*.json` |
| Intent digest | Digest over an intent document's canonical JSON. | `IntentDigest`, `VerifyIntentDigest` | `intent_sha256` |
| Partition count | Number of virtual partitions; deterministic state changes are serial per partition. | `PartitionCount` | `partition_id` |
| Tenant | The single default tenant of version 1. | `TenantID` | `tenant_id` |
| Protocol version | The runtime and worker contract identifiers. | `ProtocolVersion`, `ContractVersion`, `DeviceProtocolVersion` | `agenticstream.runtime/v1` |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Event ID prefix registry | `ids.Prefix*` | `contractsv1` kept an unused duplicate of the prefixes; `ids` is the one registry. |
