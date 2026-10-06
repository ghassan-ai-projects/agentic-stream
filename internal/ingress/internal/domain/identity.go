package domain

import "fmt"

// DefaultTenant is the tenant a source uses when none is configured.
const DefaultTenant = "default"

// TenantOrDefault names the default tenant when none is configured.
func TenantOrDefault(tenantID string) string {
	if tenantID == "" {
		return DefaultTenant
	}
	return tenantID
}

// JSONLConnectorID is the connector identity of a normalized JSONL trace.
func JSONLConnectorID(given, path string) string {
	if given == "" {
		return "jsonl:" + path
	}
	return given
}

// SimulatorConnectorID is the connector identity of a simulator trace.
func SimulatorConnectorID(given, path string) string {
	if given == "" {
		return "simulator-jsonl:" + path
	}
	return given
}

// QuarantineID scopes a raw-line quarantine record to its connector so two
// traces ingested by the same tenant cannot collide on (tenant_id, event_id).
// The quarantine table treats a same-ID/different-payload write as a conflict
// (marking the prior record rejected and erroring), so a bare "line:<N>" would
// let a second trace's malformed line at the same line number abort ingestion
// and corrupt the first record.
func QuarantineID(connectorID string, lineNum int) string {
	return fmt.Sprintf("%s:line:%d", connectorID, lineNum)
}

// LiveQuarantineID scopes a raw live line to its source instance, connection and
// line number.
func LiveQuarantineID(instanceID string, connectionID, lineNumber uint64) string {
	return fmt.Sprintf("live-uds:%s:%d:%d", instanceID, connectionID, lineNumber)
}
