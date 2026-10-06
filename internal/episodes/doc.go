// Package episodes assembles deterministic episode requests from scheduler
// items and runs bounded episode execution. The public surface is a facade:
// use cases live in internal/app, pure rules in internal/domain and every SQL
// statement in internal/store under the caller's transaction.
package episodes
