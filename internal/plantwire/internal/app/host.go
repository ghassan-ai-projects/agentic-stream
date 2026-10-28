package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

var ErrNoListener = errors.New("plantwire: the host needs a listener")

var ErrNoSiteFile = errors.New("plantwire: a host with a device catalog needs a site file")

type Config struct {
	Listener   *transport.Listener
	Settings   domain.Settings
	KindsDir   string
	SitePath   string
	Catalog    *device.CapabilityCatalog
	Authority  *authority.Service
	OwnerEpoch string
	Clock      sources.Clock
	QueueSize  int

	ReasoningInFlight ReasoningInFlight
}

type Host struct {
	listener   *transport.Listener
	settings   domain.Settings
	admission  domain.Admission
	mapping    domain.Mapping
	epoch      int64
	clock      sources.Clock
	queueSize  int
	catalog    *device.CapabilityCatalog
	authority  *authority.Service
	ownerEpoch string
	closing    atomic.Bool
	liveMu     sync.Mutex
	live       *session
	sentMu     sync.Mutex
	sent       map[string]target
	born       *bornKinds
	reasoning  ReasoningInFlight
}

func New(cfg Config) (*Host, error) {
	if err := checkConfig(cfg); err != nil {
		return nil, err
	}
	open, err := openInventory(cfg)
	if err != nil {
		return nil, err
	}
	if err := checkCatalogTargets(cfg.Catalog, open.mapping); err != nil {
		return nil, err
	}
	host := newHost(cfg, open.inventory, open.mapping)
	host.logReady(cfg, open.digest)
	return host, nil
}

func checkConfig(cfg Config) error {
	if cfg.Listener == nil {
		return ErrNoListener
	}
	if err := domain.CheckSettings(cfg.Settings); err != nil {
		return fmt.Errorf("plantwire: host settings: %w", err)
	}
	if cfg.Catalog != nil && (cfg.Authority == nil || cfg.OwnerEpoch == "") {
		return ErrAuthorityRequired
	}
	if cfg.Catalog != nil && cfg.SitePath == "" {
		return ErrNoSiteFile
	}
	return nil
}

func newHost(cfg Config, inventory domain.Inventory, mapping domain.Mapping) *Host {
	clock := sources.OrPhysical(cfg.Clock)
	epoch := cfg.Settings.Epoch
	if epoch == 0 {
		epoch = clock.Now().Unix()
	}
	admission := domain.Admission{Environment: cfg.Settings.Environment, ContractDigest: wire.ContractDigest(), Inventory: inventory}
	return &Host{listener: cfg.Listener, settings: cfg.Settings, admission: admission, mapping: mapping, epoch: epoch, clock: clock,
		queueSize: cmp.Or(max(cfg.QueueSize, 0), defaultQueueSize), catalog: cfg.Catalog,
		authority: cfg.Authority, ownerEpoch: cfg.OwnerEpoch, sent: map[string]target{}, born: &bornKinds{}, reasoning: cfg.ReasoningInFlight}
}

func (h *Host) logReady(cfg Config, siteDigest string) {
	slog.Info("plant wire host ready", "contract_version", domain.ContractVersion, "contract_digest", wire.ContractDigest(),
		"environment", cfg.Settings.Environment, "hardware_boards", cfg.Settings.HardwareBoards, "authority_epoch", h.epoch,
		"kinds_dir", cfg.KindsDir, "site", cfg.SitePath, "site_digest", siteDigest)
}

func (h *Host) Close() error {
	h.closing.Store(true)
	return h.listener.Close()
}

func (h *Host) Run(ctx context.Context, emit Emit) error {
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	queue := newIngestQueue(h.queueSize)
	fed := make(chan error, 1)
	go func() { fed <- queue.feed(ctx, emit, stop) }()
	served := h.acceptSessions(ctx, queue)
	stop(nil)
	return errors.Join(<-fed, served)
}

func (h *Host) acceptSessions(ctx context.Context, queue *ingestQueue) error {
	stop := context.AfterFunc(ctx, func() { _ = h.listener.Close() })
	defer stop()
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			return h.stopped(ctx, err)
		}
		h.serve(ctx, conn, queue)
	}
}

func (h *Host) stopped(ctx context.Context, err error) error {
	_ = h.listener.Close()
	if ctx.Err() != nil || h.closing.Load() {
		return nil
	}
	return err
}
