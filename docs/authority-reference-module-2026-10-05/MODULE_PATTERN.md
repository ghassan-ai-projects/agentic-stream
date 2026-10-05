# Module pattern

How to give another runtime module the same structure as `internal/authority`.
Apply it to modules that own durable tables and real rules. A package of pure
functions or a thin adapter does not need it.

## Layout

```
internal/<module>/
  doc.go                package comment: the business capability it owns
  api.go                aliases, constants and sentinel errors callers use
  service.go            Config, validation, New, Service
  operations.go         one delegating line per public operation
  internal/app/         use cases: checks, admission, unit of work, audit
  internal/domain/      vocabulary and rules; pure
  internal/store/       transactions and SQL for the module's tables
```

## Steps

1. **Write the language first.** List the nouns, verbs and states of the
   module with their code and column names. Record the words you retire.
   Ambiguous names are design bugs; fix them before moving code.
2. **Lock behavior.** Make sure the existing tests exercise the public
   operations, and add tests for edge cases before moving anything.
3. **Extract the store.** Move every SQL statement into `internal/store` with
   no change in meaning, as methods on a unit of work (`Tx`) opened by
   `Store.InTx`. Name them after domain actions (`RecordReboot`, not
   `UpdateRow`). The store decides nothing.
4. **Extract the domain.** Move each decision into a pure function that takes
   the loaded values and `now`, and returns a decision or an error. Remove the
   duplicate of that rule from SQL. Give it a table-driven test.
5. **Move orchestration into `internal/app`.** Each use case reads as:
   validate → unit of work → admit → load → decide → persist → audit. The
   root package keeps only configuration and one-line delegation.
6. **Replace struct literals with `New(Config)`.** Unexport fields. Make safety
   dependencies required. Remove nil guards that the constructor now
   guarantees.
7. **Narrow the public surface.** Export one `Service`, the value types callers
   pass, sentinel errors callers branch on, and stateless rules other modules
   need. Nothing else.
8. **Register the boundaries.** Add the new packages to `packageLayers` and
   `allowedImports`, point `durableOwners` at the store, and list them in the
   repository map.

## Review questions

- Can every rule be tested without a database?
- Does the root package contain anything besides configuration and delegation?
- Does any SQL statement decide something a domain function also decides?
- Can a caller build a half-configured value?
- Does a caller need to know a table or column name?
- Does each public operation read as a short sequence of domain verbs?
- Are names in code, storage and audit events the same as in the language
  table?
