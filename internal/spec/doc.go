// Package spec parses, validates, compiles and digests SituationSpec documents:
// the YAML program that declares a deployment's inputs, time policy, operators,
// situation, cognition and governed actions. The facade exposes what other
// packages use; the compiler and its rules live in internal/domain, compiling
// from a file in internal/app and the deployment and event schema SQL in
// internal/store.
package spec
