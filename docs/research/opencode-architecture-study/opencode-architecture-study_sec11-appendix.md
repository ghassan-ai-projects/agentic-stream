## Appendix A — References

**Primary source (all file:line citations refer to this revision):**

- OpenCode repository — [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode), `dev` branch @ `a19b52e85bf2` (2026-07-20), v1.18.3, MIT license. Note: the project previously lived at `sst/opencode`; GitHub redirects to the new organization.
- In-repo architecture documents: `CONTEXT.md` (v2 session-runtime domain spec) and `AGENTS.md` (layering rules, style guide) at the repository root.

**Official documentation and sites:**

- OpenCode documentation — [opencode.ai/docs](https://opencode.ai/docs/) (agents, config, permissions, plugins, MCP, server, SDK; English sources in-repo at `packages/web/src/content/docs/`)
- OpenCode 2.0 beta documentation — [v2.opencode.ai](https://v2.opencode.ai) (flagged unstable; APIs may change)
- models.dev — [models.dev](https://models.dev) (the model catalog consumed at runtime)

**Technologies referenced:**

- Vercel AI SDK v6 — [sdk.vercel.ai](https://sdk.vercel.ai) (production LLM transport)
- Effect-TS — [effect.website](https://effect.website) (v2 service layer: `Context.Service`, `Layer`, fibers, `Deferred`, `Schedule`)
- Model Context Protocol — [modelcontextprotocol.io](https://modelcontextprotocol.io) (tool/resource bridging)
- Agent Client Protocol — [agentclientprotocol.com](https://agentclientprotocol.com) (IDE/Zed integration)
- OpenTUI — [github.com/anomalyco/opentui](https://github.com/anomalyco/opentui) (terminal UI toolkit)
- tree-sitter — [tree-sitter.github.io](https://tree-sitter.github.io) (bash/PowerShell command parsing for permissions)

## Appendix B — Mermaid Diagram Sources (for web reuse)

The figures in this study were rendered as images for print fidelity. The following Mermaid sources reproduce them for websites that render Mermaid (such as the handbook at ghassan-alhamoud.com).
