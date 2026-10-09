package domain

import "testing"

func TestVersionIdentifiersAreFrozenOnTheWire(t *testing.T) {
	t.Parallel()

	if ContractVersion != "situation-runtime-contracts/v1" {
		t.Errorf("ContractVersion = %q", ContractVersion)
	}
	if ProtocolVersion != "agenticstream.runtime/v1" {
		t.Errorf("ProtocolVersion = %q", ProtocolVersion)
	}
	if DeviceProtocolVersion != 1 {
		t.Errorf("DeviceProtocolVersion = %d", DeviceProtocolVersion)
	}
	if TenantID != "default" {
		t.Errorf("TenantID = %q", TenantID)
	}
}
