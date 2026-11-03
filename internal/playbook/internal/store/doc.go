// Package store holds the playbook's transactions and every SQL statement on
// the tables it owns: rule_fires, rule_evaluations and rule_circuits. It joins
// a caller's transaction or opens its own, and reads other modules only
// through their facades.
package store
