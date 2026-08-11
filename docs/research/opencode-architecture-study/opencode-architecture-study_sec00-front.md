# OpenCode Under the Hood: Architecture and Agent Patterns of a Production Coding Agent

**A senior-architect study of the OpenCode codebase, with a blueprint for cloning the agent core**

| | |
|---|---|
| **Subject** | OpenCode — "the open source AI coding agent" |
| **Repository** | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) (formerly sst/opencode) |
| **Revision analyzed** | `dev` branch @ `a19b52e85bf2`, version 1.18.3, snapshot 2026-07-20 |
| **Companion to** | The AI Agents Patterns handbook at ghassan-alhamoud.com |
| **Scope** | The agent itself — loop, tools, models, permissions, events, extensibility. Channels (TUI, desktop, web, Slack) are analyzed only as clients of the agent core. |

*Methodology: this study is based on a full read of the source tree at the revision above, performed by seven parallel analysis tracks (agent loop, tool system, providers/models, permissions, server/events, extensibility, official documentation), cross-validated and spot-verified against the code. Every architectural claim carries an inline file:line citation that can be checked against the repository at that commit. Where the code and the documentation disagree, the code wins and the disagreement is flagged.*
