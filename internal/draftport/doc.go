// Package draftport is the contract of a rule draft: the request the playbook
// sends a worker to generalize verified cases into one rule condition, the
// reply, and the Drafter port a worker adapter implements. It holds types
// only, so a worker adapter can implement the port without reaching the
// playbook, policy or any effect.
package draftport
