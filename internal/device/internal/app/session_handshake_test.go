package app_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func TestSessionRefusesAHandshakeThatDisagreesWithTheConfiguration(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	otherDigest := "sha256:" + strings.Repeat("e", 64)
	protocolTwo := bytes.Replace(mustEncode(t, stateFor(t, catalog)), []byte(`"protocol_version":1`), []byte(`"protocol_version":2`), 1)
	cases := []struct {
		name  string
		frame []byte
		want  string
	}{
		{"wrong capability", mustEncode(t, stateFor(t, catalog, map[string]any{"capability_digest": otherDigest})), "capability digest"},
		{"wrong firmware", mustEncode(t, stateFor(t, catalog, map[string]any{"firmware_digest": otherDigest})), "firmware digest"},
		{"unsupported protocol", protocolTwo, "validate device state handshake"},
		{"not a state record", mustEncode(t, acceptedReceipt("cmd-1")), "validate device state handshake"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transport := &fakeDeviceTransport{}
			transport.queue(tc.frame)
			control := newDeviceControl(t)
			_, err := app.OpenSession(t.Context(), sessionConfig(t, transport, catalog, control.authority))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("open session error = %v, want %q", err, tc.want)
			}
			if transport.sendCount() != 0 {
				t.Fatalf("handshake failure sent %d commands", transport.sendCount())
			}
		})
	}
}

func TestSessionRefusesAnIncompleteConfiguration(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	control := newDeviceControl(t)
	cases := []struct {
		name   string
		mutate func(*app.SessionConfig)
		want   string
	}{
		{"no transport", func(c *app.SessionConfig) { c.Transport = nil }, "transport and capability catalog are required"},
		{"no catalog", func(c *app.SessionConfig) { c.Catalog = nil }, "transport and capability catalog are required"},
		{"no owner epoch", func(c *app.SessionConfig) { c.OwnerEpoch = "" }, "owner epoch is required"},
		{"no authority", func(c *app.SessionConfig) { c.Authority = nil }, "authority is required"},
		{"no firmware allow-list", func(c *app.SessionConfig) { c.AllowedFirmwareDigests = nil }, "firmware allow-list is required"},
		{"no capability allow-list", func(c *app.SessionConfig) { c.AllowedCapabilityDigests = nil }, "capability allow-list is required"},
		{"catalog not allow-listed", func(c *app.SessionConfig) {
			c.AllowedCapabilityDigests = []string{"sha256:" + strings.Repeat("e", 64)}
		}, "catalog digest is not allow-listed"},
		{"owner instance differs from the runtime owner", func(c *app.SessionConfig) { c.OwnerInstance = "instance-2" }, "owner instance must match runtime owner"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := sessionConfig(t, &fakeDeviceTransport{}, catalog, control.authority)
			tc.mutate(&config)
			_, err := app.OpenSession(t.Context(), config)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("open session error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSessionDefaultsTheOwnerInstanceToTheAuthoritys(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	control := newDeviceControl(t)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	config := sessionConfig(t, transport, catalog, control.authority)
	config.OwnerInstance = ""
	session, err := app.OpenSession(t.Context(), config)
	if err != nil {
		t.Fatalf("open session without an owner instance: %v", err)
	}
	defer func() { _ = session.Close() }()
	if session.BootID() != "boot-A" {
		t.Fatalf("session boot = %q, want boot-A", session.BootID())
	}
}

func TestSessionExposesTheHandshakeBoundIdentity(t *testing.T) {
	t.Parallel()
	session, _, catalog := openThermalSession(t)
	if got, want := session.CapabilityDigest(), catalogDigest(t, catalog); got != want {
		t.Fatalf("capability digest = %q, want %q", got, want)
	}
	if got := session.BootID(); got != "boot-A" {
		t.Fatalf("boot = %q, want boot-A", got)
	}
}

func TestANilOrClosedSessionIsNotOpen(t *testing.T) {
	t.Parallel()
	var missing *app.Session
	if missing.BootID() != "" || missing.CapabilityDigest() != "" || missing.WithTelemetry(nil) != nil {
		t.Fatal("a nil session exposed identity")
	}
	if _, err := missing.QueryState(t.Context()); err == nil || !strings.Contains(err.Error(), "device session is not open") {
		t.Fatalf("nil session query error = %v", err)
	}
	session, transport, catalog := openThermalSession(t)
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	if !transport.isClosed() {
		t.Fatal("closing the session left the gateway link open")
	}
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-closed", idemKey()))
	assertRefusedBeforeSend(t, sent, err, "device session is not open")
	if _, err := session.QueryState(t.Context()); err == nil || !strings.Contains(err.Error(), "device session is not open") {
		t.Fatalf("closed session query error = %v", err)
	}
}
