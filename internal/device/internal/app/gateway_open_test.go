package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestOpeningAGatewayEffectorRequiresALinkAndACatalog(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	for name, tc := range map[string]struct {
		config app.GatewayConfig
		want   string
	}{
		"no transport":    {app.GatewayConfig{Catalog: catalog}, "requires a device transport"},
		"no catalog":      {app.GatewayConfig{Transport: &fakeDeviceTransport{}}, "requires a capability catalog"},
		"invalid catalog": {app.GatewayConfig{Transport: &fakeDeviceTransport{}, Catalog: &domain.CapabilityCatalog{}}, "digest device capability catalog"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			effector, closeFn, err := app.OpenGatewayEffector(t.Context(), tc.config)
			if err == nil || !strings.Contains(err.Error(), tc.want) || effector != nil || closeFn != nil {
				t.Fatalf("effector=%v close=%v err=%v, want %q", effector, closeFn != nil, err, tc.want)
			}
		})
	}
}

func TestOpeningAGatewayEffectorReportsAHandshakeFailureAndClosingItClosesTheLink(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	control := newDeviceControl(t)
	config := app.GatewayConfig{
		Catalog: catalog, AllowedFirmwareDigests: []string{firmwareDigest()},
		OwnerEpoch: "epoch-1", OwnerInstance: "instance-1", Authority: control.authority,
	}

	silent := &fakeDeviceTransport{}
	config.Transport = silent
	if _, _, err := app.OpenGatewayEffector(t.Context(), config); err == nil || !strings.Contains(err.Error(), "open device session") {
		t.Fatalf("open without a handshake error = %v", err)
	}

	live := &fakeDeviceTransport{}
	live.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	config.Transport = live
	effector, closeFn, err := app.OpenGatewayEffector(t.Context(), config)
	if err != nil || effector == nil {
		t.Fatalf("open gateway effector: %v", err)
	}
	if err := closeFn(); err != nil || !live.isClosed() {
		t.Fatalf("close err=%v, link closed=%v", err, live.isClosed())
	}
}
