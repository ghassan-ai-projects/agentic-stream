# Watch reference-module migration

`internal/watch` owns derived-trigger watches: bounded, expiring conditions an approved command installs through the effect port, fired at most `max_fires` times by matching evidence, and expired without deleting audit rows. Today one `Effector` type mixes payload validation, CEL evaluation, runtime-owner and interlock SQL checks, lease-free busy retry and every SQL statement. This migration gives it the reference layers while keeping the effect-port boundary and the exact transaction behavior.

```text
internal/watch (configured facade, layer 5)
  └── internal/app (install, fire, expire use cases, layer 4)
        ├── internal/domain (payload rules, CEL, fire decisions, layer 2)
        └── internal/store (opaque transactions and watch SQL, layer 3)
```

Merge decision: watch stays its own module. It cannot move into `actions` (the architecture forbids `actions` reaching concrete effectors) or into `engine`/`runtime` (reasoning and replay layers must not reach effect implementations), and it is too cohesive to split. The new layers are kept thin: no `wire` package, because the only codec is a four-case integer coercion that belongs in domain.
