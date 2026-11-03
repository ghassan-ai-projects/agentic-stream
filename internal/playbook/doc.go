// Package playbook owns rule promotion's runtime: which approved playbook rule
// answers an admitted episode instead of the model, the audits that re-ask the
// model about a rule-handled case, the shadow evaluations of rules not yet
// active, and each rule's circuit, which switches it off until a human closes
// it. Service is the only entry point; it delegates to the use cases in
// internal/app, the pure rules in internal/domain and the tables of
// internal/store. See documentation/design/playbook.md.
package playbook
