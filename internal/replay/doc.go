// Package replay provides deterministic, effect-safe replay of a trace
// against a spec: it verifies the canonical situation-version hashes, and in
// its worker-aware modes verifies recorded decisions, compares shadow trials
// report-only, or simulates counterfactual commands. The facade delegates to
// the module's app use cases; verification rules live in
// internal/replay/internal/domain, SQL in internal/store, and file and
// database resources in internal/transport.
package replay
