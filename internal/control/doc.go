// Package control owns the runtime control plane: singleton runtime ownership,
// policy-epoch drain and kill, the read-only final dispatch readiness
// capability, and durable aggregate cost control (ceilings, the kill switch,
// reservations and settlements).
package control
