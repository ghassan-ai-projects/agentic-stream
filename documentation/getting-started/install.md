# Install and build

Audience: contributors and evaluators. Scope: build the current Go checkout and
run the repository's local quality gates.

## Requirements

- Go `1.26.5` or a compatible Go 1.26 toolchain.
- A Unix-like environment for the default Unix domain socket worker boundary.
- `make` for the repository's standard build and quality targets.
- Optional: `protoc` 35.1 and the pinned Go generators when changing the worker
  protocol or running `make proto-check`.

The exact module and dependency versions are in [`go.mod`](../../go.mod).

## Build from a checkout

```bash
make build
./bin/agentic-stream version
```

The build writes the binary to `bin/agentic-stream` by default. The version
command prints build metadata plus the current contract and protocol versions.

## Run the baseline checks

For a fast local check:

```bash
go test ./...
go vet ./...
git diff --check
```

For the repository's CI-equivalent local gate:

```bash
make ci-check
```

`ci-check` also verifies generated worker stubs, module tidiness, linting,
short race-enabled tests, dead-code checks when installed, and vulnerability
checks when installed. See [testing reference](../reference/testing.md) for
the full target list.

## Generated protocol code

The committed Go files under `proto/agenticstream/runtime/v1/` are generated
from [`docs/design/contracts/runtime-v1.proto`](../../docs/design/contracts/runtime-v1.proto).
Do not hand-edit them. When the protocol changes, use:

```bash
make proto-generate
make proto-check
```

Protocol changes require compatibility review and updates to the public
[worker contract](../contracts/worker-protocol.md).

## Next reads

- [Quickstart](quickstart.md)
- [Compatibility](../overview/compatibility.md)
- [Quality gates](../governance/quality.md)
