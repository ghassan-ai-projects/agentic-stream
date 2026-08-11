# New Feature Prompt

Add the feature only after clarifying the existing structure and constraints.

Process:

1. Identify the affected package boundaries, commands, docs, and tests.
2. Find the simplest design that fits existing architecture.
3. Add or update tests first when changing production behavior.
4. Implement the feature without unrelated refactors.
5. Update README, `AGENTS.md`, or context files if the workflow or expectations changed.
6. Validate with the relevant build, test, vet, and lint commands.

Reject shortcuts that leave the repository in a state future agents cannot understand.
