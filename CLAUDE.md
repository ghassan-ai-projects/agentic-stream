@AGENTS.md

## Claude Code Notes

- `.claude/rules/` holds thin path-scoped pointers to `.agents/context/`, so the matching rules arrive when you open a matching file. Keep the content in `.agents/context/`, where every agent can read it.
- Personal preferences go in `CLAUDE.local.md` (git-ignored).
