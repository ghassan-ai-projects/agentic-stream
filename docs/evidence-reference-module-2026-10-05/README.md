# Evidence reference-module migration

Evidence is the capability-scoped, bounded, read-only tool boundary for episode
workers. Its public package becomes a configured Service facade. Application
use cases order token admission, durable reservation, query and completion;
domain rules decide authorization and lifecycle validity; store owns SQL;
wire owns token/argument/result codecs; transport adapts gRPC to application
values. No protocol, schema, digest or external effect changes are planned.

```mermaid
flowchart TD
    C[Runtime and remote executor] --> F[Service facade]
    F --> A[app: issue, verify, admit, reserve, query, complete, recover]
    F --> T[transport: gRPC adaptation]
    T --> A
    A --> D[domain: scope and lifecycle rules]
    A --> S[store: opaque transactions and ledger SQL]
    A --> W[wire: tokens, arguments, fingerprint and result codecs]
    S --> D
    W --> D
```

- [Findings](FINDINGS.md)
- [Language](UBIQUITOUS_LANGUAGE.md)
- [Design](DESIGN.md)
- [Plan](PLAN.md)
