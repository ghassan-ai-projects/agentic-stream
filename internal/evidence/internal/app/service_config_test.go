package app

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func validCapabilityConfig() *CapabilityConfig {
	return &CapabilityConfig{Issuer: "runtime", Audience: "tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte(testKey)}}
}

func noopQuery(context.Context, Call) (QueryResult, error) { return QueryResult{}, nil }

func TestNewRefusesConfigurationThatLacksASafetyDependency(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	capabilities, err := New(Config{Capabilities: validCapabilityConfig()})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := New(Config{Ledger: ledgerOn(db, allowOwner, "epoch-1")})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		cfg  func() Config
		want string
	}{
		{"nothing configured", func() Config { return Config{} }, "configuration is required"},
		{"no issuer", func() Config { c := validCapabilityConfig(); c.Issuer = ""; return Config{Capabilities: c} }, "issuer, audience and signing key"},
		{"no audience", func() Config { c := validCapabilityConfig(); c.Audience = ""; return Config{Capabilities: c} }, "issuer, audience and signing key"},
		{"no signing key id", func() Config { c := validCapabilityConfig(); c.KeyID = ""; return Config{Capabilities: c} }, "issuer, audience and signing key"},
		{"short signing key", func() Config {
			c := validCapabilityConfig()
			c.Keys["k1"] = []byte("short")
			return Config{Capabilities: c}
		}, "issuer, audience and signing key"},
		{"short key elsewhere in the ring", func() Config {
			c := validCapabilityConfig()
			c.Keys["k0"] = []byte("short")
			return Config{Capabilities: c}
		}, "key is too short"},
		{"negative lifetime", func() Config { c := validCapabilityConfig(); c.MaxTTL = -1; return Config{Capabilities: c} }, "must not be negative"},
		{"negative skew", func() Config { c := validCapabilityConfig(); c.ClockSkew = -1; return Config{Capabilities: c} }, "must not be negative"},
		{"ledger without a database", func() Config { return Config{Ledger: &Ledger{LeaseOwner: "owner", RuntimeEpoch: "epoch-1"}} }, "ledger database, owner check"},
		{"ledger without an owner check", func() Config {
			l := ledgerOn(db, nil, "epoch-1")
			return Config{Ledger: l}
		}, "ledger database, owner check"},
		{"ledger without an epoch", func() Config { return Config{Ledger: ledgerOn(db, allowOwner, "")} }, "ledger database, owner check"},
		{"ledger without a lease owner", func() Config {
			l := ledgerOn(db, allowOwner, "epoch-1")
			l.LeaseOwner = ""
			return Config{Ledger: l}
		}, "ledger database, owner check"},
		{"calls without capabilities", func() Config {
			return Config{Calls: &CallConfig{Ledger: ledger, Query: noopQuery, RuntimeEpoch: "epoch-1"}}
		}, "calls require"},
		{"calls with a ledger-only capability service", func() Config {
			return Config{Calls: &CallConfig{Capabilities: ledger, Ledger: ledger, Query: noopQuery, RuntimeEpoch: "epoch-1"}}
		}, "calls require"},
		{"calls without a ledger", func() Config {
			return Config{Calls: &CallConfig{Capabilities: capabilities, Query: noopQuery, RuntimeEpoch: "epoch-1"}}
		}, "calls require"},
		{"calls with a capability-only ledger", func() Config {
			return Config{Calls: &CallConfig{Capabilities: capabilities, Ledger: capabilities, Query: noopQuery, RuntimeEpoch: "epoch-1"}}
		}, "calls require"},
		{"calls without a query", func() Config {
			return Config{Calls: &CallConfig{Capabilities: capabilities, Ledger: ledger, RuntimeEpoch: "epoch-1"}}
		}, "calls require"},
		{"calls without an epoch", func() Config {
			return Config{Calls: &CallConfig{Capabilities: capabilities, Ledger: ledger, Query: noopQuery}}
		}, "calls require"},
		{"calls whose epoch differs from the ledger's", func() Config {
			return Config{Calls: &CallConfig{Capabilities: capabilities, Ledger: ledger, Query: noopQuery, RuntimeEpoch: "epoch-2"}}
		}, "does not match call epoch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, err := New(test.cfg())
			if service != nil {
				t.Fatal("a service was built from an unsafe configuration")
			}
			requireContains(t, err, test.want)
		})
	}
}

func TestNewCopiesTheKeyRingSoLaterEditsCannotChangeAuthority(t *testing.T) {
	t.Parallel()
	cfg := validCapabilityConfig()
	cfg.Now = fixedClock(testNow)
	service, err := New(Config{Capabilities: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Keys["k1"] {
		cfg.Keys["k1"][i] = 0
	}
	delete(cfg.Keys, "k1")
	scope := grantedScope()
	scope.Traceparent = testTraceparent
	token, err := service.Issue(scope)
	if err != nil {
		t.Fatalf("Issue after the caller edited its key ring: %v", err)
	}
	if _, err := service.Verify(token); err != nil {
		t.Fatalf("Verify after the caller edited its key ring: %v", err)
	}
}

func TestUnconfiguredOperationsRefuseInsteadOfSkippingASafetyCheck(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	ledgerOnly, err := New(Config{Ledger: ledgerOn(db, allowOwner, "epoch-1")})
	if err != nil {
		t.Fatal(err)
	}
	capabilityOnly := capabilityService(t, testNow)
	t.Run("a ledger-only service cannot issue or verify", func(t *testing.T) {
		t.Parallel()
		_, err := ledgerOnly.Issue(grantedScope())
		requireContains(t, err, "issuance is not configured")
		_, err = ledgerOnly.Verify(nil)
		requireContains(t, err, "verification is not configured")
	})
	t.Run("a capability-only service cannot serve calls or touch the ledger", func(t *testing.T) {
		t.Parallel()
		_, err := capabilityOnly.Call(t.Context(), workerEnvelope(nil, "call-1"))
		requireRefusal(t, err, domain.FailedPrecondition)
		requireContains(t, capabilityOnly.ReclaimExpired(t.Context(), testNow), "ledger is not configured")
		_, err = capabilityOnly.RecoverTx(t.Context(), nil, testNow)
		requireContains(t, err, "recovery is not configured")
	})
	t.Run("a ledger-only service cannot serve calls", func(t *testing.T) {
		t.Parallel()
		_, err := ledgerOnly.Call(t.Context(), workerEnvelope(nil, "call-1"))
		requireRefusal(t, err, domain.FailedPrecondition)
	})
}

func TestServiceReportsItsRuntimeBindingAndClock(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	ledger, err := New(Config{Ledger: ledgerOn(db, allowOwner, "epoch-1")})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := capabilityService(t, testNow)
	if ledger.RuntimeEpoch() != "epoch-1" || capabilities.RuntimeEpoch() != "" {
		t.Fatalf("epochs = %q and %q, want epoch-1 and none", ledger.RuntimeEpoch(), capabilities.RuntimeEpoch())
	}
	if !ledger.JoinRecovery().Configured() || capabilities.JoinRecovery().Configured() {
		t.Fatal("only the ledger service joins recovery transactions")
	}
	if got := capabilities.IssueTime(); !got.Equal(testNow) {
		t.Fatalf("IssueTime() = %v, want the configured clock %v", got, testNow)
	}
	if got := ledger.IssueTime(); time.Since(got) > time.Minute || got.Location() != time.UTC {
		t.Fatalf("IssueTime() without a configured clock = %v, want the current time in UTC", got)
	}
}
