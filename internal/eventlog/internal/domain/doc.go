// Package domain holds the event log's pure rules: registered-schema payload
// checking, quarantine identity derivation, event body encoding, stored-time
// decoding and use-case input validation. Rules operate on plain values; time
// arrives as data, and there is no I/O, no clock read and no contract package
// import, which keeps the module below its low-level importers.
package domain
