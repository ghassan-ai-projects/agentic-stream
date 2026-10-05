# Module pattern

How to give another runtime module the same structure as `internal/authority`.
Apply it to modules that own durable tables and real rules. A package of pure
functions or a thin adapter does not need it.

## Layout

```
internal/<module>/
  doc.go                package comment: the business capability it owns
  <module>.go           Service, Config, New; aliases and sentinel errors
  tx.go                 withAdmittedTx / withPriorityTx (or the module's equivalents)
  <operation group>.go  one file per group of public operations
  internal/domain/      vocabulary and rules; pure
  internal/store/       SQL for the module's tables
```

## Steps

1. **Write the language first.** List the nouns, verbs and states of the
   module with their code and column names. Record the words you retire.
   Ambiguous names are design bugs; fix them before moving code.
2. **Lock behavior.** Make sure the existing tests exercise the public
   operations, and add tests for edge cases before moving anything.
3. **Extract the store.** Move every SQL statement into `internal/store` with
   no change in meaning. Mutations take `*sql.Tx`; reads take a `Reader`.
   Store functions are named after domain actions (`RecordReboot`, not
   `UpdateRow`).
4. **Extract the domain.** Move each decision into a pure function that takes
   the loaded values and `now`, and returns a decision or an error. Remove the
   duplicate of that rule from SQL. Give it a table-driven test.
5. **Reduce the root package to orchestration.** Each public operation reads as:
   validate → transaction → admit → load → decide → persist → audit.
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
- Does any SQL statement decide something a domain function also decides?
- Can a caller build a half-configured value?
- Does a caller need to know a table or column name?
- Does each public operation read as a short sequence of domain verbs?
- Are names in code, storage and audit events the same as in the language
  table?
