# worker

Status: done
Round: 10

`internal/worker` holds the worker protocol: constants and rules (`internal/domain`),
the private Unix-socket transport (`internal/transport`), and a facade. The fake
worker lives in `testsupport/workerfake` (see [testsupport.md](testsupport.md)).

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top / sub) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `worker` (facade) | 100.0% | 100.0% | 1.3 s | 1.3 s | 2 / 0 | 3 / 3 |
| `worker/internal/domain` | 100.0% | 100.0% | 1.3 s | 1.3 s | 2 / 0 | 3 / 15 |
| `worker/internal/transport` | **69.4%** | 78.8% | 1.3 s | 1.3 s | 5 / 0 | 7 / 7 |

No test takes more than 0.01 s. Test-hygiene findings: 5 (`usetesting` on `os.MkdirTemp` three times, missing `t.Parallel` twice), 0 after.

The 21% of `transport` still uncovered are operating-system failure branches
(`chmod`, `stat` or `Lstat` failing after success, a created directory whose mode
needs correcting under an unusual umask). They cannot be provoked without
replacing the file system.

## Findings and changes

### Removed
- `TestListenEvidenceSocketRefusesAStaleSocket` and `TestListenEvidenceSocketRefusesAnActiveOrUnsafePath`: merged into one table, `TestListenEvidenceSocketRefusesAnExistingPathItDoesNotOwn` (live socket, stale socket, regular file, symlink).
- In the facade `TestFacadeListensAndDialsPrivateSockets`: its one-entry `map` of dialers and its "dial then close, assert nothing" body proved only that a lazy dial does not fail. Replaced by two end-to-end tests that answer a handshake over the socket.

### Renamed or moved
- `TestFacadeExposesProtocolRules` → `TestFacadeExposesTheProtocolRules` (now pins the frozen wire values `1.0`, `1.0`, `evidence_tools.v1`, 4096 events, 16 MiB, which are a wire contract, not a literal-equals-literal test).
- `TestBudgetRequiresAPositiveWallTime` → `TestBudgetRequiresAValidPositiveWallTime` (table); `budget_test.go` split from `protocol_test.go`.

### Improved
- T6: all tests parallel; the shared `privateDir` helper (one `//nolint:usetesting` with the macOS 104-byte socket path reason) replaces two inline `os.MkdirTemp` copies; the facade uses `workerfake.SocketDir`.
- T4: socket-path and budget tests assert the message that tells the two budget faults apart ("requires a valid wall_time" vs "must be positive") and the single socket-path message.

### Added
- `TestWorkerSocketWithTLSAdmitsOnlyAClientThatTrustsTheServer`: a worker served over TLS on the private socket is reachable by a client that trusts its certificate, and refused for a plain client and for a client that trusts nobody. This is the only test of the TLS branch of `DialEpisodeWorkerSocketTLS` (previously the TLS config was passed but never used).
- `TestWorkerAnswersOverThePrivateSocketItIsDialledOn`: a handshake through `ListenEvidenceSocket` + `DialEpisodeWorkerSocketTLS`.
- transport: `TestListenEvidenceSocketRefusesAParentThatIsNotPrivate` (0755 directory, symlinked directory, regular file), `TestListenEvidenceSocketCreatesAMissingParentPrivately`, `TestClosingTheListenerToleratesAnAlreadyRemovedSocket`, a symlink to a live socket in the refusal table.
- domain: the clean-path rule gets the redundant-separator and trailing-separator cases; the budget rule gets a malformed duration and a one-nanosecond wall time.

### Speed
- Nothing was slow.

## Production code touched
- none

## Invariants proven here
- 6 (worker boundary, no credentials or effectors reachable): the evidence socket is the only runtime-owned surface a worker reaches. It is private and not replaceable: `TestEvidenceSocketIsPrivateAndCleansUp` (0600 socket, 0700 parent), `TestListenEvidenceSocketRefusesAParentThatIsNotPrivate`, `TestListenEvidenceSocketRefusesAnExistingPathItDoesNotOwn` (no takeover of a live socket, regular file or symlink), `TestClosingTheListenerKeepsAReplacedSocket` (cleanup never deletes a file the listener did not create), `TestSocketPathRuleAcceptsOnlyCleanAbsoluteUnixPaths` (no TCP or `unix://` endpoint), `TestBudgetRequiresAValidPositiveWallTime` (a worker request always carries a finite wall time).
- 5: `TestBudgetRequiresAValidPositiveWallTime`.

## Open items
- `ListenEvidenceSocket` is used for the worker socket too in tests; the name says "evidence". Naming only, not changed.
- `refuseExistingEvidenceSocket` probes an existing socket with a real 100 ms dial timeout. The tests hit it with an immediately refused connection, so the timeout is never waited on.
