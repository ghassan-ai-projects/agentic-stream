// Package wire is the vendored plant wire contract and its record codec: it
// embeds the contract's schemas and pinned digest, decodes one record per line
// within the contract's size limits, validates it against the schema of its
// type, names the members a record fails at, keeps 64-bit integers exact, and
// parses kind files. It decides nothing beyond well-formedness.
package wire
