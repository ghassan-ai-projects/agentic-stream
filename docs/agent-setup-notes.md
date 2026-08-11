# Agent Setup Notes

This repository follows the patterns that have become common across high-profile coding-agent ecosystems:

- Keep one canonical repository instruction file and make tool-specific files delegate to it.
- Treat Codex as the native `AGENTS.md` consumer; do not create a duplicate `CODEX.md`.
- Keep always-loaded instructions concise, specific, and testable.
- Put commands, architecture boundaries, security rules, and handoff expectations in durable files.
- Use hooks, CI, and linters for enforcement because instruction files guide behavior but do not guarantee it.
- Prefer narrower, path-scoped instructions or custom agents for specialized workflows.
- Preinstall or document dependencies so cloud agents can build and test without trial and error.
- Add governance files (`CONTRIBUTING.md`, `SECURITY.md`, PR templates, issue templates) so agents and humans share the same review ritual.

Useful public references:

- Claude Code memory and `CLAUDE.md`: https://code.claude.com/docs/en/memory
- Gemini CLI configuration and context files: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/configuration.md
- GitHub Copilot coding agent best practices: https://docs.github.com/en/copilot/tutorials/cloud-agent/get-the-best-results
- OpenAI Codex `AGENTS.md` guidance: https://developers.openai.com/codex/guides/agents-md
- OpenAI Codex configuration: https://developers.openai.com/codex/config-basic

Maintenance guidance:

- Update `AGENTS.md` when review feedback reveals a repeated mistake.
- Remove stale instructions quickly; conflicting rules are worse than missing rules.
- Keep bridge files short. If a bridge file grows, ask whether the rule belongs in `AGENTS.md`, a path-scoped agent rule, or a workflow-specific custom agent.
- Keep Codex settings separate from Codex guidance: `.codex/config.toml` is for behavior/configuration, while `AGENTS.md` is for durable repository instructions.
- Audit `make ci-check`, `.github/workflows/ci.yml`, and the Quality Gates table together so the prose and automation stay aligned.
- Keep CI tool versions pinned. Avoid `@latest` in critical pipelines unless the job is explicitly experimental.
