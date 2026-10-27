package plantwire

import (
	"context"
	"net"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
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
	ErrAuthorityRequired      = app.ErrAuthorityRequired
	ErrNoSiteFile             = app.ErrNoSiteFile
)

// Errors of a command: no live session, an unborn device, a reboot since the
// command was bound, an open reconciliation of the device, or a hardware board
// the host may not command (nothing sent), no site device for the command's
// target, a missing final authorization check, a node refusal (the receipt's
// reason is in the provider result) and a failure the device reported. An
// unknown outcome is an actionport.UnknownOutcomeError.
var (
	ErrNoSession              = domain.ErrNoSession
	ErrOriginNotAuthorized    = domain.ErrOriginNotAuthorized
	ErrDeviceNotBorn          = domain.ErrDeviceNotBorn
	ErrBootChanged            = domain.ErrBootChanged
	ErrReconciliationRequired = authority.ErrReconciliationRequired
	ErrDeviceNotInSite        = app.ErrDeviceNotInSite
	ErrAuthorizationRequired  = app.ErrAuthorizationRequired
	ErrCommandRefused         = app.ErrCommandRefused
	ErrCommandFailed          = app.ErrCommandFailed
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
// A host with a catalog needs Authority, the device authority of the runtime
// owner, and OwnerEpoch, the owner epoch it commands under. Clock defaults to
// the physical clock.
type Config struct {
	Listener   net.Listener
	Settings   Settings
	KindsDir   string
	SitePath   string
	Catalog    *device.CapabilityCatalog
	Authority  *authority.Service
	OwnerEpoch string
	Clock      sources.Clock
}

// Host serves one plant wire session at a time on its listener.
type Host struct{ app *app.Host }

// New checks the settings, commissions the kind files and reads the site file.
func New(cfg Config) (*Host, error) {
	host, err := app.New(app.Config{Listener: transport.NewListener(cfg.Listener), Settings: cfg.Settings,
		KindsDir: cfg.KindsDir, SitePath: cfg.SitePath, Catalog: cfg.Catalog, Authority: cfg.Authority, OwnerEpoch: cfg.OwnerEpoch,
		Clock: cfg.Clock})
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

// BornKindDigests reports, per kind name, the kind digest the latest birth
// announced, whether or not a kind file commissions that digest. Rule
// promotion reads it to switch off rules pinned to another digest.
func (h *Host) BornKindDigests() map[string]string { return h.app.BornKindDigests() }

// EntityKind is the kind of the site device that reports for an entity; empty
// when no site device reports for it.
func (h *Host) EntityKind(entityID string) string { return h.app.EntityKind(entityID) }

// Close closes the listener, so no further session is accepted; a running Run
// returns when the caller's context ends.
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
// through the site file, runs the final authorization check, reads the
// device's current boot and origin from the live session, and refuses a device
// on a hardware board unless the host holds physical-actuation consent and the
// board is one of its hardware boards (ErrOriginNotAuthorized, with the
// provider result refused origin_not_authorized and the board). It records the
// boot with the device authority (a reboot opens a reconciliation); while a
// reconciliation is open it presents the device's latest readings of its
// outputs to the authority, which clears it when each output has a good, live
// reading on the boot newer than the opening, and refuses the command
// otherwise. It then claims the target, binds the command and
// asserts the claim before it sends one DCMD: the command's idempotency key unchanged, the
// device's current boot, a deadline of the route's expiry from now on the
// host clock, the host's authority epoch, the policy digest and safe_off on
// lease loss. It waits for the terminal answer: accepted then succeeded is
// pending verification (never done on the device's word), a refusal, a
// reported failure or a command still queued when the session ends
// (session_ending) is a failure, and a result unknown to the device, a lost
// session after the command was sent, or no terminal answer before the
// deadline or ctx is an unknown outcome. An unknown outcome opens a
// reconciliation for the device boot, and an executed command whose claim is
// no longer held becomes one; the session's claims are released when it ends.
func (h *Host) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	return h.app.DispatchAuthorized(ctx, command, authorization)
}

// VerifyDeviceCommand verifies a command this host sent that succeeded or
// whose outcome is unknown, from the commanded device's own readings: every
// argument named like a writable metric of the device's kind must be read back
// on that metric (within the metric's resolution; a word exactly) in a good,
// live sample on the command's boot that arrives after the command's result,
// within VerificationWindow of the dispatch's return. Verified, it reports
// succeeded with the readings sealed as device-state evidence, and the next
// HSYNC carries the numeric observations as command_evidence. Otherwise it
// gives no final status, so the command stays pending verification, and a
// lapsed window is sent as not_observed.
func (h *Host) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	return h.app.VerifyDeviceCommand(ctx, command)
}

// VerificationWindow is how long a command waits for the readings that verify
// it: one report period of the slowest output in use (the twin's indicator
// reports every 5 s) and a second of slack. Only a matching reading verifies,
// so a reading after a lease has lapsed cannot verify a leased command.
const VerificationWindow = app.VerificationWindow

// AnswerGrace is how long the host waits for the node's terminal answer after
// the command's own deadline.
const AnswerGrace = app.AnswerGrace
