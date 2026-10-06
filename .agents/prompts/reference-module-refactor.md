# Reference Module Refactor Prompt

Rebuild one package to the reference-module standard set by `internal/authority`
(see `docs/authority-reference-module-2026-10-05/`). Use this prompt when asked
to "do the same" for another package, to make a package a reference
implementation, or to separate its layers.

The user's standing preferences for this work:

- Plan first. Write findings and a plan into a dated folder under `docs/`
  before touching code.
- One responsibility per layer. Never put logic in the persistence or adapter
  layer, never put database or transport code in the logic layer, and keep the
  public package a thin facade.
- Name things in the domain's own language and keep code, storage and audit
  names identical.
- Backward compatibility is not required unless the user says so: rename and
  update callers instead of adding shims.
- Explain trade-offs in software-engineering terms. Repository ADRs may be
  wrong; do not argue from them.

## 1. Survey (read before deciding anything)

1. Read every production file of the package.
2. List the public surface that other packages actually use:

   ```bash
   grep -rhoE '\b<pkg>\.[A-Z][A-Za-z]*' --include='*.go' . | grep -v '^./internal/<pkg>/' | sort | uniq -c | sort -rn
   grep -rln 'internal/<pkg>"' --include='*.go' . | grep -v '^./internal/<pkg>/'
   ```

   Separate production callers from tests. Exported symbols used only by tests
   are not public API.
3. List cross-module data access in both directions: SQL against tables the
   package does not own, and other packages querying its tables
   (`grep -rn '<table>' --include='*.go'`).
4. Read the package's tests and the architecture gates that name it
   (`architecture_test.go`, `architecture_flow_test.go`,
   `architecture_ownership_test.go`, `architecture_layers_test.go`).

## 2. Choose the layer shape

Pick the shape from what the package does, not from habit.

| Package kind | Layers |
| --- | --- |
| Owns durable tables and rules | facade · `internal/app` · `internal/domain` · `internal/store` |
| Talks to external systems (devices, network, files), no tables of its own | facade · `internal/app` · `internal/domain` · one adapter package per external system (for example `internal/wire`, `internal/transport`) |
| Pure rules, no I/O | facade · `internal/domain`, or a single package when it is small |

Layer responsibilities, enforced by architecture tests:

- **Facade** (`internal/<pkg>`): public types (aliases of domain values),
  `New(Config)` with required safety dependencies, one documented line per
  operation delegating to `app`. No logic, no SQL, no transactions.
- **app**: use cases. Validate input → open the unit of work → admission or
  other cross-module checks → load → decide (domain) → persist or send
  (adapter) → audit. Must not import `database/sql` or `internal/storage`.
- **domain**: vocabulary and every decision as pure functions over values.
  Time arrives as a parameter. No I/O, no clock reads, no imports of
  `database/sql`, `net`, `os`, `storage`, `control` or `clock`.
- **store / adapters**: the only place that touches SQL, sockets, files or
  wire encodings. Methods are named after domain actions. They decide nothing:
  a `WHERE` clause or a frame check selects data; it never decides whether an
  operation is allowed. Running another module's transactional check on an open
  transaction is plumbing (`Tx.Assert(ctx, fence, epoch)`), not logic.

## 3. Write the planning folder

Create `docs/<pkg>-reference-module-<YYYY-MM-DD>/` with:

| File | Contents |
| --- | --- |
| `README.md` | One-paragraph summary and the target layer diagram. |
| `FINDINGS.md` | Problems with file references, grouped by principle: mixed responsibilities, leaking public surface, optional safety dependencies, cross-module data access, vocabulary drift, untyped records, smaller defects. |
| `internal/<pkg>/UBIQUITOUS_LANGUAGE.md` | Written in the package, not in the dated folder (the folder's README links to it). Tables of terms: meaning, code name, storage or wire name. A "retired words" table with the replacement and why. An architecture gate requires it for every layered module. |
| `DESIGN.md` | Layers, responsibilities and "must not" per layer, public API table, rules every operation follows, enforcement table, schema or wire changes. |
| `PLAN.md` | Rounds with proof per round, behavior that must not change, deliberate behavior changes, deferred follow-ups, and a status table filled in as rounds land. |

Commit this folder as round 0 before writing code.

## 4. Implement in rounds

Each round: focused tests, `gofmt`, `golangci-lint run ./...`, commit. Never
loosen a lint threshold to pass; split functions instead (15 body lines, one
level of abstraction).

1. **Shared helpers out.** Move utilities that are not the package's business
   (digests, encodings) to their foundation package.
2. **Domain.** Create `internal/<pkg>/internal/domain` with types and pure rules
   and table-driven tests. Register it in `packageLayers`, `allowedImports` and
   `documentation/architecture/repository-map.md` in the same commit.
3. **Store or adapters.** Move every SQL statement, socket and codec into the
   adapter layer, named after domain actions. Point `durableOwners` at the
   store.
4. **App and facade.** Move orchestration into `internal/app`; reduce the root
   package to configuration and delegation. Update every caller and test. When
   ownership gates would see two writers, land store and app in one commit.
5. **Gates and docs.** Add or extend architecture tests, prove each new gate by
   injecting a violation and watching it fail, then update `AGENTS.md`,
   `.agents/context/architecture-bar.md` and the module maps.
6. **Polish rounds**, one commit each:
   - typed records instead of `map[string]any`, parsed once at the boundary
     with a closed field set, keeping the original document when a digest
     depends on its exact bytes;
   - ports for cross-module reads, so each module reads only its own tables;
   - request values for operations with more than about four inputs;
   - decisions that drifted into `app` moved back to `domain`.

## 5. Preserve behavior

Do not change, unless the plan lists the change as deliberate:

- error precedence (which check fails first), sentinel errors and `errors.Is`
  behavior;
- identity and digest inputs, golden replay fixtures, and wire formats;
- transaction boundaries (a state change and its audit commit together),
  locks, clock reads, cancellation and fencing;
- fail-closed behavior: a missing dependency is a constructor error, never a
  silently skipped check.

Record every deliberate behavior change in `PLAN.md`.

## 6. Pitfalls seen before

- Tests that set `db.SetMaxOpenConns(1)` deadlock if they query the database
  while a unit of work holds the connection. Read results after the unit of
  work returns.
- `golangci-lint` `wrapcheck` accepts errors from a module's own
  `internal/*/internal/*` layers and from `storage.DB.WithTx`; wrap everything
  else with the operation name. Do not add `fmt.Errorf("%w", err)` no-ops.
- Each new package needs its own tests at 60% coverage or more; behavior tests
  belong to the layer they exercise, and the facade keeps configuration and
  delegation tests.
- A function value can satisfy a port without the caller importing the port's
  package (`app.Fences{RuntimeOwner: owner.Assert}`), which keeps layers from
  importing each other sideways.
- The `proto-check` target needs the pinned `protoc`; when the machine has a
  different version, run the other `make ci-check` targets individually and say
  so.

## 7. Report and rate

End with a short summary, then rate the package out of 10 across: layering,
domain rules, fail-closed safety, ubiquitous language, tests, encapsulation at
the data level, type safety and simplicity. List the remaining weaknesses in
order and what would raise the score.
