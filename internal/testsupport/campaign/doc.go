// Package campaign drives agentic-stream through a streams-simulator campaign
// for rule promotion: it prepares each run's session plan, answers episodes
// and rule drafts like Tamoz from canned pump answers (recorded, so a campaign
// replays without the model), approves through the signed approval endpoint
// as a scripted approver, writes each run's consumer verdict and gathers the
// simulator's grades and this repository's ledgers into a campaign score.
// It is test support: only tests import it.
package campaign
