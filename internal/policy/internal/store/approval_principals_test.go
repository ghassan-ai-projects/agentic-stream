package store

import (
	"testing"
	"time"
)

func TestApprovalAuthorityNeedsAnActiveRoleGrantForTheEntityAndRisk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change string
		tenant string
		risk   string
		want   int
	}{
		{name: "granted", want: 1},
		{name: "another risk class", risk: "R1", want: 0},
		{name: "another tenant", tenant: "other", want: 0},
		{name: "the approver was disabled", change: "UPDATE principals SET status = 'disabled' WHERE principal_id = 'operator-1'", want: 0},
		{name: "the role was unbound", change: "DELETE FROM principal_roles", want: 0},
		{name: "the grant was revoked", change: "DELETE FROM approval_authorities", want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
			if test.change != "" {
				if _, err := db.ExecContext(t.Context(), test.change); err != nil {
					t.Fatal(err)
				}
			}
			inJoined(t, db, func(tx *Tx) error {
				row, err := tx.LoadIntent(t.Context(), intentID)
				if err != nil {
					return err
				}
				if test.tenant != "" {
					row.TenantID = test.tenant
				}
				if test.risk != "" {
					row.RiskClass = test.risk
				}
				entity, err := tx.ApprovalEntity(t.Context(), row.SituationID)
				if err != nil || entity != "motor-1" {
					t.Fatalf("entity = %q, %v", entity, err)
				}
				if got, err := tx.ApprovalAuthority(t.Context(), row, entity, "operator-1"); err != nil || got != test.want {
					t.Fatalf("authority grants = %d, %v; want %d", got, err, test.want)
				}
				return nil
			})
		})
	}
}

func TestRelayAndApproverReadsSeeOnlyActivePrincipalsOfTheTenant(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		if active, err := tx.RelayActivity(t.Context(), "tenant", "relay-1"); err != nil || active != 1 {
			t.Errorf("active relay = %d, %v", active, err)
		}
		if active, err := tx.RelayActivity(t.Context(), "other", "relay-1"); err != nil || active != 0 {
			t.Errorf("relay of another tenant = %d, %v", active, err)
		}
		if key, err := tx.ApproverKey(t.Context(), "tenant", "operator-1"); err != nil || len(key) != 32 {
			t.Errorf("approver key = %d bytes, %v", len(key), err)
		}
		if key, err := tx.ApproverKey(t.Context(), "other", "operator-1"); err == nil {
			t.Errorf("approver key of another tenant = %d bytes, want a refusal", len(key))
		}
		return nil
	})
	if _, err := db.ExecContext(t.Context(), "UPDATE principals SET status = 'disabled'"); err != nil {
		t.Fatal(err)
	}
	inJoined(t, db, func(tx *Tx) error {
		if active, err := tx.RelayActivity(t.Context(), "tenant", "relay-1"); err != nil || active != 0 {
			t.Errorf("disabled relay = %d, %v", active, err)
		}
		if _, err := tx.ApproverKey(t.Context(), "tenant", "operator-1"); err == nil {
			t.Error("a disabled approver still has a verification key")
		}
		return nil
	})
}
