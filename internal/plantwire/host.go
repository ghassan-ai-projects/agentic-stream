package plantwire

import (
	"context"
	"net"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// Environments a host accepts: a simulated or hardware host accepts only a
// node of that environment, a mixed host accepts all three.
const (
	EnvironmentSimulated = domain.EnvironmentSimulated
	EnvironmentHardware  = domain.EnvironmentHardware
	EnvironmentMixed     = domain.EnvironmentMixed
)

// Errors of the host settings, the kind files and the listener.
var (
	ErrUnknownEnvironment     = domain.ErrUnknownEnvironment
	ErrHardwareWithoutConsent = domain.ErrHardwareWithoutConsent
	ErrBoardsOnSimulatedHost  = domain.ErrBoardsOnSimulatedHost
	ErrBoardName              = domain.ErrBoardName
	ErrBoardTwice             = domain.ErrBoardTwice
	ErrNegativeEpoch          = domain.ErrNegativeEpoch
	ErrNoListener             = app.ErrNoListener
)

// Errors of a command: no live session or an unborn device (nothing sent), no
// site device for the command's target, a missing final authorization check, a
// node refusal (the receipt's reason is in the provider result) and a failure
// the device reported. An unknown outcome is an actionport.UnknownOutcomeError.
var (
	ErrNoSession             = domain.ErrNoSession
	ErrDeviceNotBorn         = domain.ErrDeviceNotBorn
	ErrDeviceNotInSite       = app.ErrDeviceNotInSite
	ErrAuthorizationRequired = app.ErrAuthorizationRequired
	ErrCommandRefused        = app.ErrCommandRefused
	ErrCommandFailed         = app.ErrCommandFailed
)

// Settings are the host's environment, the hardware boards it may command,
// whether the process holds physical-actuation consent, and its authority
// epoch (0: Unix seconds when the host starts).
type Settings = domain.Settings

// Config opens a host. Listener is an owner-only Unix listener the caller
// created; the host closes it when it stops. KindsDir holds the commissioned
// kind files (*.kind.json). SitePath is the optional site file: checked
// against its schema and the commissioned kinds, and digested. Without it no
// reading becomes an event. Catalog is the optional capability catalog whose
// routes the host owns as an effector; without it the host commands nothing.
// Clock defaults to the physical clock.
type Config struct {
	Listener net.Listener
	Settings Settings
	KindsDir string
	SitePath string
	Catalog  *device.CapabilityCatalog
	Clock    sources.Clock
}

// Host serves one plant wire session at a time on its listener.
type Host struct{ app *app.Host }

// New checks the settings, commissions the kind files and reads the site file.
func New(cfg Config) (*Host, error) {
	host, err := app.New(app.Config{Listener: transport.NewListener(cfg.Listener), Settings: cfg.Settings,
		KindsDir: cfg.KindsDir, SitePath: cfg.SitePath, Catalog: cfg.Catalog, Clock: cfg.Clock})
	if err != nil {
		return nil, err
	}
	return &Host{app: host}, nil
}

// CheckSettings refuses an unknown environment, a negative epoch, a board name
// that is not a plant wire id or appears twice, hardware boards on a simulated
// host, and a hardware or mixed host without physical-actuation consent.
func CheckSettings(settings Settings) error { return domain.CheckSettings(settings) }

// Run accepts node sessions until ctx ends, then says goodbye to the live
// session, waits for it to end and closes the listener. The readings and
// heartbeats the site file maps become normalized events handed to emit in
// order, from a queue beside the session, so a slow emit never delays a
// barrier; a full queue ends the session. An emit error stops the host and is
// returned.
func (h *Host) Run(ctx context.Context, emit func(context.Context, contractsv1.Envelope) error) error {
	return h.app.Run(ctx, emit)
}

// Close closes the listener; a running Run then returns.
func (h *Host) Close() error { return h.app.Close() }

// OwnsRoute reports whether the catalog names route; the watch route is never
// a device's.
func (h *Host) OwnsRoute(route string) bool { return h.app.OwnsRoute(route) }

// Dispatch refuses: a plant wire command is sent only after the final
// authorization check, through DispatchAuthorized.
func (h *Host) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	return h.app.Dispatch(ctx, command)
}

// DispatchAuthorized bounds the command with the catalog, resolves its device
// through the site file, runs the final authorization check and sends one
// DCMD to the live session: the command's idempotency key unchanged, the
// device's current boot, a deadline of the route's expiry from now on the
// host clock, the host's authority epoch, the policy digest and safe_off on
// lease loss. It waits for the terminal answer: accepted then succeeded is
// pending verification (never done on the device's word), a refusal, a
// reported failure or a command still queued when the session ends
// (session_ending) is a failure, and a result unknown to the device, a lost
// session after the command was sent, or no terminal answer before the
// deadline or ctx is an unknown outcome.
func (h *Host) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	return h.app.DispatchAuthorized(ctx, command, authorization)
}

// VerifyDeviceCommand gives no final status yet: a succeeded command stays
// pending verification until readings confirm it.
func (h *Host) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	return h.app.VerifyDeviceCommand(ctx, command)
}
