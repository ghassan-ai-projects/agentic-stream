// Package rule is the in-process executor of playbook rules. A request the
// playbook bound to a rule carries the rule's recommendation; the executor
// turns it into a typed Decision (the rule's diagnosis code, one intent with
// the rule's preset, the features it read as facts) without a model call, so
// the decision takes the ordinary validation, policy and dispatch path. Every
// other request goes to the model executor it wraps.
package rule
