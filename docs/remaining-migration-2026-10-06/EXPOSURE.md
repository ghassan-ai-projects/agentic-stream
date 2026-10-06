# Public surface per migrated module

The facade of a module exposes only what other packages use. Exported symbols that nothing outside the module uses stay inside the domain layer; they are listed here as candidates for deletion, for the owner to decide. Nothing is deleted by the migrations.

## contractsv1

Exposed to other packages: 24 symbols used by production code, plus 4 used only by other packages' tests (`ConformanceValidFrame, SchemaDeviceReceipt, SchemaDeviceResult, SchemaDeviceState`).

Exported in the domain but **not exposed** (no other package uses them; candidates for deletion or for staying internal): `ClassificationConfidential, ClassificationPublic, ClassificationRestricted, ConformanceInvalidFrames, ConformanceValidMessageTypes, InvalidFrame, PartitionCount, PayloadHash, SchemaID, SchemaTriggerEvaluation, SpanLink`.

