package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestEpochRefusalsByState(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		state                      string
		decision, ordinary, admits error
	}{
		{"", nil, errors.New("unknown"), nil},
		{EpochDraining, nil, ErrEpochDraining, ErrEpochDraining},
		{EpochKilled, ErrEpochKilled, ErrEpochKilled, ErrEpochDraining},
	} {
		t.Run(test.state, func(t *testing.T) {
			t.Parallel()
			if err := RefuseDecision(test.state); !errors.Is(err, test.decision) {
				t.Errorf("decision = %v, want %v", err, test.decision)
			}
			err := RefuseOrdinary(test.state)
			if test.state == "" {
				if err == nil || !strings.Contains(err.Error(), "unknown epoch control state") {
					t.Errorf("ordinary = %v", err)
				}
			} else if !errors.Is(err, test.ordinary) {
				t.Errorf("ordinary = %v, want %v", err, test.ordinary)
			}
			if err := RefuseAdmission(test.state); !errors.Is(err, test.admits) {
				t.Errorf("admission = %v, want %v", err, test.admits)
			}
		})
	}
}

func TestOnlyDrainAndKillAreControllable(t *testing.T) {
	t.Parallel()
	for state, ok := range map[string]bool{EpochDraining: true, EpochKilled: true, "": false, "paused": false} {
		if (CheckControllable(state) == nil) != ok {
			t.Errorf("CheckControllable(%q) ok=%v, want %v", state, !ok, ok)
		}
	}
}

func TestOwnerRules(t *testing.T) {
	t.Parallel()
	if err := CheckHolder(Holder{"e", "i"}, "e", "i"); err != nil {
		t.Fatalf("own lease refused: %v", err)
	}
	for _, other := range []Holder{{"x", "i"}, {"e", "x"}} {
		if err := CheckHolder(other, "e", "i"); !errors.Is(err, ErrRuntimeOwnerBusy) {
			t.Fatalf("holder %+v: %v", other, err)
		}
	}
	for rows, ok := range map[int64]bool{0: false, 1: true, 2: false} {
		if (CheckOwnerMutation(rows) == nil) != ok {
			t.Errorf("rows=%d accepted=%v", rows, !ok)
		}
	}
}

func TestCostRequestValidation(t *testing.T) {
	t.Parallel()
	if CheckReservation("e", "t", 0, "now") != nil || CheckSettlement("e", 0, "now") != nil || CheckLimit("global", 0, "now") != nil {
		t.Fatal("valid request refused")
	}
	for name, err := range map[string]error{
		"reservation no episode": CheckReservation("", "t", 1, "now"),
		"reservation no tenant":  CheckReservation("e", "", 1, "now"),
		"reservation no time":    CheckReservation("e", "t", 1, ""),
		"reservation too large":  CheckReservation("e", "t", math.MaxUint64, "now"),
		"settlement no episode":  CheckSettlement("", 1, "now"),
		"settlement too large":   CheckSettlement("e", math.MaxUint64, "now"),
		"limit no scope":         CheckLimit("", 1, "now"),
		"limit too large":        CheckLimit("global", math.MaxUint64, "now"),
	} {
		if err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCostAdmissionDecisions(t *testing.T) {
	t.Parallel()
	if CheckNoActiveCeiling(0, false) != nil {
		t.Fatal("inactive ceiling refused")
	}
	for _, active := range []struct {
		max  int64
		kill bool
	}{{1, false}, {0, true}} {
		if err := CheckNoActiveCeiling(active.max, active.kill); !errors.Is(err, ErrCostReservationRejected) {
			t.Errorf("active ceiling %+v: %v", active, err)
		}
	}
	if err := CheckReserved(0, "global"); !errors.Is(err, ErrCostReservationRejected) || !strings.HasSuffix(err.Error(), ": global") {
		t.Fatalf("unreserved = %v", err)
	}
	if CheckReserved(1, "global") != nil || RefuseMissingLimit(TenantScope("t")) != nil || RefuseMissingLimit(GlobalScope) == nil {
		t.Fatal("missing-limit rules changed")
	}
	if CheckLimitWritten(1) != nil || CheckLimitWritten(0) == nil || CheckLimitSettled(1, "x") != nil || CheckLimitSettled(0, "x") == nil {
		t.Fatal("row count rules changed")
	}
}

func TestSettlementIsIdempotentOnlyForTheSameActual(t *testing.T) {
	t.Parallel()
	open := Reservation{Status: ReservationReserved}
	if needed, err := open.NeedsSettlement("e", 5); !needed || err != nil {
		t.Fatalf("open = %v %v", needed, err)
	}
	settled := Reservation{Status: ReservationSettled, Actual: 5}
	if needed, err := settled.NeedsSettlement("e", 5); needed || err != nil {
		t.Fatalf("repeat = %v %v", needed, err)
	}
	if _, err := settled.NeedsSettlement("e", 6); err == nil {
		t.Fatal("different actual accepted")
	}
	if _, err := (Reservation{Status: "other"}).NeedsSettlement("e", 0); err == nil {
		t.Fatal("unknown status accepted")
	}
}

func TestStoredLimitsAndCeilingMerge(t *testing.T) {
	t.Parallel()
	if _, err := CheckStoredLimit("global", -1); err == nil {
		t.Fatal("negative stored limit accepted")
	}
	if value, err := CheckStoredLimit("global", 7); err != nil || value != 7 {
		t.Fatalf("value=%d err=%v", value, err)
	}
	limit, kill := uint64(30), false
	ceilings := CostCeilings{Global: &limit, KillSwitch: &kill}
	if maxMicro, killSwitch := ceilings.MergeGlobal(100, true); maxMicro != 30 || killSwitch {
		t.Fatalf("merged = %d %v", maxMicro, killSwitch)
	}
	if maxMicro, killSwitch := (CostCeilings{}).MergeGlobal(100, true); maxMicro != 100 || !killSwitch {
		t.Fatalf("unset merge = %d %v", maxMicro, killSwitch)
	}
	if !(CostCeilings{}).Empty() || ceilings.Empty() || (CostCeilings{}).ChangesGlobal() || !ceilings.ChangesGlobal() || ceilings.TenantKillSwitch() || (CostCeilings{}).TenantKillSwitch() {
		t.Fatal("ceiling predicates changed")
	}
}
