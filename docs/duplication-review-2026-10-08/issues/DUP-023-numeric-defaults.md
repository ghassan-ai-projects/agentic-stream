# DUP-023: One-minute lease, capability TTL, evidence read budget and tenant default stated in several modules

- Status: fixed
- Severity: low
- Verdict (finders): REAL
- Themes: business rules, mechanisms
- Wave: 2
- Finder sources: R9, M8 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Leases come from `sources.OrLease`/`DefaultLease`; keep authority's negative-lease rejection. Remove remote's redundant 15 minute fallback after confirming `PrepareScope` defaults it. Do not merge the unrelated 15 minute scheduler expiry.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report R9: Numeric defaults repeated under different names (leases, capability TTL, evidence read budget)

- Verdict: REAL
- Shared meaning: one-minute lease default; 15-minute capability lifetime; default evidence read budget (1000 rows, 1 MiB, 24 h window).
- Sites (lease, "1 minute"): internal/sources/sources.go:99-106 (`DefaultLease`, `OrLease`, the consolidated one); internal/control/internal/domain/owner.go:11-20 (`DefaultLease = time.Minute` + `LeaseDuration`: independent copy of `OrLease`); internal/authority/service.go:19,74-79 (`defaultClaimLease = time.Minute`; `claimLease()` treats 0 as default and negative as an error, `OrLease` treats <=0 as default); internal/runtime/internal/composition/planes.go:147 (`LeaseFor: time.Minute`, redundant with `OrLease` inside actions/internal/app/service.go:54); cmd/agentic-stream/serve.go:48 (flag default `time.Minute`) and cmd/agentic-stream/run_live.go:83 (`time.Minute` literal passed as owner lease); internal/evidence/internal/domain/deadline.go:9 (`now.Add(time.Minute)` call deadline, different concept, same number).
- Sites (capability TTL, "15 minutes"): internal/evidence/internal/domain/capability.go:10 (`DefaultCapabilityTTL`); internal/evidence/internal/app/capability.go:13 (`defaultCapabilityTTL = domain.DefaultCapabilityTTL`, the same constant under a second name in the same module); internal/executor/remote/internal/app/capability.go:38-41 (`expiresAt = now.Add(15 * time.Minute)`, redundant because `PrepareScope` already defaults `ExpiresAt` to `IssuedAt+maxTTL`; and wrong if an issuer is configured with a shorter `MaxTTL`, since `CheckIssuable` rejects lifetime > maxTTL). Unrelated 15m (scheduler item expiry): internal/cognition/internal/domain/timing.go:13 - do not merge.
- Sites (evidence read budget): internal/runtime/internal/transport/worker.go:179-180 (`From: now-24h, Until: now+24h, MaxRows: 1000, MaxBytes: 1<<20`); internal/executor/native/internal/store/evidence_tool.go:28 (`maxRows: 1000, maxBytes: 1 << 20`); internal/executor/native/internal/domain/evidence_query.go:46 (`now.Add(-24 * time.Hour)`).
- How they differ: authority rejects a negative claim lease where the others coerce it; the remote worker window reaches 24 h into the future where native stops at now; otherwise same numbers.
- Risk if left: a lease policy change (for example 30 s) is applied to some owners and not others; capability TTL change leaves the remote issuer on 15 m.
- Proposed canonical owner: leases: `internal/sources.DefaultLease/OrLease` (control, authority, runtime/composition can import sources; cmd already does); capability TTL: evidence domain constant, remote deletes its fallback and leaves `ExpiresAt` zero; evidence budget: define once in the `evidence` facade (`evidence.DefaultReadBudget`) and have runtime transport and native consume it (native may only import evidence via the existing `episodes -> evidence` edge, check architecture_native_executor_test.go before wiring).
- Proposed fix: delete `control.DefaultLease/LeaseDuration` and `authority.defaultClaimLease` in favour of `sources.OrLease`; delete `LeaseFor: time.Minute` in planes.go and the `time.Minute` flag default literals by referencing `sources.DefaultLease`; drop the duplicate const alias in evidence/app; remove remote's 15m fallback.
- Behaviour to preserve: authority's negative-lease validation error; lease values in tests; token validity windows.
- Verification: control owner tests, authority config tests, evidence capability tests, remote capability tests. New: a test asserting the default lease constants come from sources.

### Finder report M8: Defaults for tenant, lease and capability TTL stated in several modules

- Verdict: REAL
- Shared meaning: one default value per concept (tenant "default", runtime lease 1 minute, capability lifetime 15 minutes).
- Sites (tenant): contractsv1/internal/domain/declarations.go:4 `TenantID = "default"` (used only by engine/config.go:54-55); ingress/internal/domain/identity.go:6-12 `DefaultTenant`/`TenantOrDefault`; runtime/internal/composition/planes.go:26-27 literal; cmd flags: cmd/agentic-stream/run_command.go:33, run_live.go:33, operator.go:26, serve.go:46 (`"default"` four times).
- Sites (lease): sources/sources.go:98-106 `DefaultLease`/`OrLease` (<= 0 -> default); control/internal/domain/owner.go:11-21 `DefaultLease`/`LeaseDuration` (identical function and value); authority/service.go:19,74-78 `defaultClaimLease` (== 0 -> default; negative rejected at :69); runtime composition planes.go (`LeaseFor: time.Minute`); cmd/agentic-stream/serve.go:48 and run_live.go:83 (`time.Minute` literals); cmd/operator.go:17 `operatorLease = 30s`.
- Sites (capability TTL): evidence/internal/domain/capability.go:10 `DefaultCapabilityTTL = 15m` (the verifier's maximum); executor/remote/internal/app/capability.go:40 `now.Add(15 * time.Minute)` (the issuer's default expiry, must stay <= that maximum).
- Sites (string default): actions/internal/app/service.go:57 `orDefault` duplicates stdlib `cmp.Or` that evidence already uses (capability_verify.go:45).
- How they differ: no divergence in value today; the coupling (issuer default <= verifier maximum) and the two lease helpers are independent edits waiting to diverge; authority's helper differs on negative input (validated elsewhere).
- Risk if left: changing the default tenant or lease in one place leaves the CLI, composition and ingress disagreeing.
- Proposed canonical owner: tenant -> `contractsv1.TenantID` (cmd, composition and ingress already may import contractsv1; ingress/internal/domain imports it, cmd imports it); lease -> `sources.OrLease` (control's domain has no imports, so control uses `sources` from control/internal/app or store, or keeps `LeaseDuration` as the one caller of `sources.OrLease`); capability TTL -> export `evidence.DefaultCapabilityTTL` from the evidence facade and use it in remote (remote/internal/app already imports evidence).
- Proposed fix: replace the literals/helpers with the owners above; replace `orDefault` with `cmp.Or`; flag defaults use the constants (`contractsv1.TenantID`, `sources.DefaultLease`).
- Behaviour to preserve: `serve --owner-lease` default 1m, `--tenant` default "default", operator lease 30 s (intentionally shorter), 15-minute capability default.
- Verification: cmd main_test/live_test flag-default tests, evidence capability tests, control owner tests. New test: a parity test in composition asserting `ingress` default tenant, engine default tenant and `contractsv1.TenantID` are equal until merged.

## Outcome

Verified (re-read every site):
- Confirmed: `control.DefaultLease/LeaseDuration` was an identical copy of `sources.OrLease`; `authority.defaultClaimLease/claimLease` duplicated it (the negative-lease rejection lives in `Config.validate`, so `sources.OrLease` after validation is equivalent); `LeaseFor: time.Minute` in composition was redundant with `sources.OrLease` in actions; `serve.go`/`run_live.go` lease literals; evidence `defaultCapabilityTTL` alias; four `"default"` tenant flag literals, the composition literal, and ingress `DefaultTenant` (a copy of `contractsv1.TenantID`); the 1000 rows / 1 MiB / 24 h evidence budget in runtime transport and native; `actions.orDefault` duplicated `cmp.Or`.
- Confirmed latent bug: remote's `now.Add(15 * time.Minute)` fallback ignored the issuer's `MaxTTL`; with `MaxTTL < 15m` a default-expiry token would be refused by `CheckIssuable`. `PrepareScope` already defaults `ExpiresAt` to `IssuedAt+maxTTL`, so the fallback was deleted and the default now follows the issuer's maximum.
- Partly right: the "15 minute scheduler expiry" and `evidence` call deadline (`time.Minute`) are different concepts and were left alone, as were the operator lease (30 s, intentional) and the engine's `contractsv1.TenantID` fallback (already the owner). The remote worker window reaching 24 h into the future versus native stopping at now is intentional and kept; only the shared 24 h span and the row/byte budget are shared constants.

Changed:
- Leases: deleted `control/internal/domain` `DefaultLease`/`LeaseDuration` (control app uses `sources.OrLease`); deleted `authority` `defaultClaimLease`/`claimLease()` (and replaced the private `clock()` helper with `sources.OrPhysical`); removed `LeaseFor: time.Minute` from `runtime/internal/composition/planes.go`; `serve --owner-lease` default and `run-live` owner lease use `sources.DefaultLease`.
- Tenant: `ingress/internal/domain` `DefaultTenant` deleted, `TenantOrDefault` is `cmp.Or(tenantID, contractsv1.TenantID)`; composition and the four cmd `--tenant` flags use `contractsv1.TenantID`.
- Capability TTL: evidence app alias removed (uses `domain.DefaultCapabilityTTL`); remote issuer no longer sets its own 15 minute expiry and `issueScopedCapability` was folded into `Issue`.
- Evidence budget: new `evidence.DefaultReadMaxRows`, `DefaultReadMaxBytes`, `DefaultReadWindow` (defined in evidence domain, re-exported by the facade). Runtime transport uses them through the new `evidenceCapabilityFactory`; native store uses them and passes the window through a new `EvidenceScope.Window` field (so native's pure domain does not import evidence).
- `actions/internal/app` `orDefault` replaced by `cmp.Or`.
- Files: architecture_test.go (allowedImports: control/internal/app -> sources, composition -> contractsv1, native/internal/store -> evidence), cmd/agentic-stream/{operator,run_command,run_live,serve}.go, internal/{control,authority,actions,evidence,ingress,runtime,executor/native,executor/remote} files listed in git status for those trees.

Pinning tests: `TestRuntimeOwnerUnsetLeaseIsTheSourcesDefaultLease` (control), `TestUnsetClaimLeaseIsTheSourcesDefaultLease` and `TestNewRefusesConfigurationThatSkipsSafetyChecks/negative_lease` (authority), `TestFlagDefaultsComeFromTheirOwners` (cmd, tenant and owner-lease flag defaults), `TestPipelineDefaultsTenantIsTheContractTenant` (composition), the `TenantOrDefault` assertion in ingress `rules_test.go`, now against `contractsv1.TenantID`, `TestAttemptCapabilityDefaultExpiryFollowsTheIssuerMaximum` (remote; regression for the MaxTTL bug), `TestEvidenceCapabilityFactoryUsesTheEvidenceReadBudget` (runtime transport), `TestEvidenceToolStartsWithTheEvidenceReadBudget` (native store), `TestEvidenceQueryWindowDefaultsToTheScopeWindowBeforeNow` (native domain). The removed control assertion on `LeaseDuration` is covered by `TestOrLeaseDefaultsAnUnspecifiedLease` (sources).

Deferred: none.
