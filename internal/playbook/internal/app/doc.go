// Package app orders the playbook use cases: route an episode admission to a
// rule, an audit or the model inside the admission's transaction, and settle
// the evidence (queue due audits, judge pending evaluations, open circuits on
// a disagreeing audit).
package app
