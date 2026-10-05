// Package watch owns derived-trigger watches: bounded, expiring conditions
// installed by an approved command through the effect port, fired at most
// max_fires times by matching evidence, and expired without deleting audit
// rows. It never reaches dispatch, policy or reasoning. See the
// [module guide](README.md).
package watch
