package runtime

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestCostConfigurationPreservesUnspecifiedLimitsAndAtomicity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                      string
		global, tenant                            *uint64
		kill                                      *bool
		wantGlobal, wantTenant                    uint64
		wantGlobalKill, wantTenantKill, wantError bool
	}{
		{"tenant only", nil, costUint(20), nil, 100, 20, true, false, false},
		{"global only keeps kill", costUint(30), nil, nil, 30, 10, true, true, false},
		{"kill only keeps ceilings", nil, nil, costBool(false), 100, 10, false, true, false},
		{"both with kill", costUint(30), costUint(20), costBool(false), 30, 20, false, false, false},
		{"tenant failure rolls back global", costUint(30), costUint(math.MaxUint64), costBool(false), 100, 10, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "limits.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				if err := costcontrol.SetLimit(ctx, tx, "global", "", 100, true, "2026-01-01T00:00:00Z"); err != nil {
					return err
				}
				return costcontrol.SetLimit(ctx, tx, "tenant:tenant", "tenant", 10, true, "2026-01-01T00:00:00Z")
			}); err != nil {
				t.Fatal(err)
			}
			err = configureCostLimits(ctx, PipelineConfig{DB: db, TenantID: "tenant", Clock: clock.NewVirtual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), GlobalCostCeiling: tc.global, TenantCostCeiling: tc.tenant, CostKillSwitch: tc.kill})
			if (err != nil) != tc.wantError {
				t.Fatalf("configuration err=%v", err)
			}
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				for _, want := range []struct {
					scope   string
					ceiling uint64
					kill    bool
				}{{"global", tc.wantGlobal, tc.wantGlobalKill}, {"tenant:tenant", tc.wantTenant, tc.wantTenantKill}} {
					ceiling, kill, err := readCostLimit(ctx, tx, want.scope)
					if err != nil {
						return err
					}
					if ceiling != want.ceiling || kill != want.kill {
						t.Errorf("%s ceiling=%d kill=%v want=%d/%v", want.scope, ceiling, kill, want.ceiling, want.kill)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func costUint(value uint64) *uint64 { return &value }
func costBool(value bool) *bool     { return &value }
