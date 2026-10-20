package plantwire

import (
	"context"
	"net"

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

// Settings are the host's environment, the hardware boards it may command,
// whether the process holds physical-actuation consent, and its authority
// epoch (0: Unix seconds when the host starts).
type Settings = domain.Settings

// Config opens a host. Listener is an owner-only Unix listener the caller
// created; the host closes it when it stops. KindsDir holds the commissioned
// kind files (*.kind.json). SitePath is the optional site file, read and
// digested. Clock defaults to the physical clock.
type Config struct {
	Listener net.Listener
	Settings Settings
	KindsDir string
	SitePath string
	Clock    sources.Clock
}

// Host serves one plant wire session at a time on its listener.
type Host struct{ app *app.Host }

// New checks the settings, commissions the kind files and reads the site file.
func New(cfg Config) (*Host, error) {
	host, err := app.New(app.Config{Listener: transport.NewListener(cfg.Listener), Settings: cfg.Settings,
		KindsDir: cfg.KindsDir, SitePath: cfg.SitePath, Clock: cfg.Clock})
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
// session, waits for it to end and closes the listener.
func (h *Host) Run(ctx context.Context) error { return h.app.Run(ctx) }

// Close closes the listener; a running Run then returns.
func (h *Host) Close() error { return h.app.Close() }
