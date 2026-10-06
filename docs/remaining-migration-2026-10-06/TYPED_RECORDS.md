# Typed records rounds

Goal: replace `map[string]any` documents with typed records parsed once at the
boundary, with a closed field set, keeping the original bytes wherever a digest
depends on them. One commit per round, each with focused tests, lint and the
architecture gates. Order is by untyped-map count and by how self-contained the
document is. Rules for every round: no digest, identity or wire change; error
precedence unchanged unless listed; the original document is kept when a digest
is computed over its exact form.

| Round | Target | Maps before | Status |
| --- | --- | --- | --- |
