// Package policy governs accepted intents and records signed human approvals.
//
// The public Service is a configured facade. Its internal/app layer sequences
// evaluation and approval use cases, internal/domain owns pure governance rules
// and typed documents, and internal/store owns SQL and lifecycle plumbing.
//
// Every operation joins the caller's transaction. Ownership, decision epoch and
// action-readiness checks are required configuration; consequential intents are
// automated only under an active calibration artifact the store reads, and
// otherwise go to human approval.
// Decision and intent documents retain their original canonical digest inputs.
//
// README.md describes the module pattern. UBIQUITOUS_LANGUAGE.md maps the
// package's values to durable governance records.
package policy
