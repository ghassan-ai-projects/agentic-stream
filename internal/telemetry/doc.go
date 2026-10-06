// Package telemetry contains the runtime's low-cardinality operational
// measurements. It is intentionally provider-neutral; deployments can bridge
// the snapshot to OpenTelemetry or Prometheus without changing stream logic. The
// facade exposes the counters, the metrics handler and the tracing helpers;
// counters live in internal/domain and the OpenTelemetry and HTTP adapters in
// internal/transport.
package telemetry
