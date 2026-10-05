// Package device is the effect boundary to physical devices: effect-profile
// policy, capability catalog loading, the device gateway link, and the
// gateway, simulated and fail-closed effectors. It is a thin facade over
// internal/app (session use cases and effectors), internal/domain (catalog,
// materialization and protocol rules), internal/wire (record codec) and
// internal/transport (Unix-socket gateway link).
package device
