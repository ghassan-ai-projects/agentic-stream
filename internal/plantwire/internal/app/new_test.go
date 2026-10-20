package app_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func TestNewRefusesAHostItCannotServeSafely(t *testing.T) {
	t.Parallel()
	kinds := plantwirefake.KindsDir(t, ".")
	cases := []struct {
		name   string
		change func(*app.Config)
		want   func(error) bool
	}{
		{"no listener", func(c *app.Config) { c.Listener = nil }, is(app.ErrNoListener)},
		{"an unknown environment", func(c *app.Config) { c.Settings.Environment = "real" }, is(domain.ErrUnknownEnvironment)},
		{"a mixed host without consent", func(c *app.Config) { c.Settings.Environment = domain.EnvironmentMixed }, is(domain.ErrHardwareWithoutConsent)},
		{"a missing kind directory", func(c *app.Config) { c.KindsDir = filepath.Join(kinds, "absent") }, is(os.ErrNotExist)},
		{"a kind directory without kind files", func(c *app.Config) { c.KindsDir = t.TempDir() }, is(app.ErrNoKindFiles)},
		{"a kind file that fails its schema", func(c *app.Config) { c.KindsDir = kindsWith(t, "broken.kind.json", `{"kind":"pump"}`) }, isSchemaRefusal},
		{"one kind in two files", func(c *app.Config) { c.KindsDir = kindsWith(t, "again.kind.json", kindFile(t, kinds)) }, is(domain.ErrKindCommissionedTwice)},
		{"an unreadable site file", func(c *app.Config) { c.SitePath = filepath.Join(kinds, "absent.site.json") }, is(os.ErrNotExist)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := app.Config{Listener: listener(t), Settings: simulatedHost, KindsDir: kinds}
			tc.change(&config)
			_, err := app.New(config)
			if !tc.want(err) {
				t.Fatalf("New = %v, want the refusal of %s", err, tc.name)
			}
		})
	}
}

func TestNewReadsTheSiteFileAndDefaultsTheEpochToTheClock(t *testing.T) {
	t.Parallel()
	site := filepath.Join(t.TempDir(), "bench.site.json")
	if err := os.WriteFile(site, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := simulatedHost
	settings.Epoch = 0
	host, err := app.New(app.Config{Listener: listener(t), Settings: settings, KindsDir: plantwirefake.KindsDir(t, "."), SitePath: site})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
}

func is(target error) func(error) bool {
	return func(err error) bool { return errors.Is(err, target) }
}

func isSchemaRefusal(err error) bool {
	var refused *wire.SchemaError
	return errors.As(err, &refused)
}

func listener(t *testing.T) *transport.Listener {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return transport.NewListener(listener)
}

func kindsWith(t *testing.T, name, content string) string {
	t.Helper()
	dir := plantwirefake.KindsDir(t, ".")
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func kindFile(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "pump.kind.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
