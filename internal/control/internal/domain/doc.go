// Package domain holds runtime-control vocabulary and rules as pure functions:
// owner lease rules, epoch drain/kill refusals, and cost reservation,
// settlement and ceiling decisions. It performs no I/O and reads no clock;
// time arrives as a parameter.
package domain
