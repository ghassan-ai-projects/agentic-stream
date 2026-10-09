// Package architecture holds the repository-wide architecture and quality
// gates. It has no production code: every gate is a test that reads the
// repository's Go sources, configuration and documentation and fails when a
// rule of AGENTS.md, the quality bar or the architecture bar is broken. The
// README indexes the gates.
package architecture
