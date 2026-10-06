// Package domain holds the SituationSpec rules as pure code: parsing, JSON Schema
// validation, normalization, reference resolution, CEL expression checks, duration
// parsing, canonical sealing with the spec digest, and the embedded event schema
// registry. It performs no I/O and reads no clock.
package domain
