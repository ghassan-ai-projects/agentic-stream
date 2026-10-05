// Package app orders the action plane's use cases: lease one approved command,
// re-prove its authority, call the effector outside any transaction, verify the
// device state when the effector can, and record the outcome atomically. It
// decides through domain and persists through store; it holds no SQL.
package app
