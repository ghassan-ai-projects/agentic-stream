// Package domain holds the device-authority vocabulary and rules: owners,
// device boots, target claims and fences, command bindings, reconciliation,
// safe-stop latching and safety evidence. Every rule is a pure function over
// values; time arrives as a parameter, and nothing here reads a clock, opens a
// transaction or performs I/O.
package domain
