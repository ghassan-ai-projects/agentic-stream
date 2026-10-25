package domain

import "fmt"

const (
	RefusalProtocolInvalid     = "protocol_invalid"
	RefusalEnvironmentMismatch = "environment_mismatch"
	RefusalContractUnsupported = "contract_unsupported"
	RefusalVersionUnsupported  = "version_unsupported"
	RefusalInventoryMismatch   = "inventory_mismatch"
)

type Admission struct {
	Environment    string
	ContractDigest string
	Inventory      Inventory
}

type Verdict struct {
	Refusal         string
	Reason          string
	ProtocolVersion int64
}

func (v Verdict) Accepted() bool { return v.Refusal == "" }

func (a Admission) Admit(b Birth) Verdict {
	for _, rule := range []func(Birth) Verdict{protocolValid, a.environmentAgrees, a.contractSupported, versionSupported, a.inventoryMatches} {
		if verdict := rule(b); !verdict.Accepted() {
			return verdict
		}
	}
	return Verdict{ProtocolVersion: ProtocolVersion}
}

func protocolValid(b Birth) Verdict {
	if b.ProtocolMin <= b.ProtocolMax {
		return Verdict{}
	}
	return refuse(RefusalProtocolInvalid, "protocol_min %d is above protocol_max %d", b.ProtocolMin, b.ProtocolMax)
}

func (a Admission) environmentAgrees(b Birth) Verdict {
	fault := faultOfEnvironment(a.Environment, b)
	if fault == nil {
		return Verdict{}
	}
	return refuse(RefusalEnvironmentMismatch, "%s: %s", fault.pointer, fault.reason)
}

func (a Admission) contractSupported(b Birth) Verdict {
	if b.ContractVersion == ContractVersion && b.ContractDigest == a.ContractDigest {
		return Verdict{}
	}
	return refuse(RefusalContractUnsupported, "the host speaks contract %s %s, the node announces %s %s",
		ContractVersion, a.ContractDigest, b.ContractVersion, b.ContractDigest)
}

func versionSupported(b Birth) Verdict {
	if b.ProtocolMin <= ProtocolVersion && ProtocolVersion <= b.ProtocolMax {
		return Verdict{}
	}
	return refuse(RefusalVersionUnsupported, "the host speaks protocol %d, the node %d to %d", ProtocolVersion, b.ProtocolMin, b.ProtocolMax)
}

func (a Admission) inventoryMatches(b Birth) Verdict {
	if kind, wrong := misdigestedKind(b); wrong {
		return refuse(RefusalInventoryMismatch, "kind %q announces a kind_digest its entry does not give", kind)
	}
	if kind, twice := repeatedKind(b); twice {
		return refuse(RefusalInventoryMismatch, "kind %q is announced twice", kind)
	}
	if device, missing := a.Inventory.missingDevice(b); missing {
		return refuse(RefusalInventoryMismatch, "commissioned device %q is not in the birth", device)
	}
	return Verdict{}
}

func refuse(refusal, format string, args ...any) Verdict {
	return Verdict{Refusal: refusal, Reason: fmt.Sprintf(format, args...)}
}
