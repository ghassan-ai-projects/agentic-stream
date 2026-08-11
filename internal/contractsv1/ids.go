// Package contractsv1 holds the public domain types and codecs for the v1 runtime.
package contractsv1

import "fmt"

// ID is a runtime identifier with a known prefix space.
type ID string

// Prefix constants mirror those in internal/ids for validation and parsing.
const (
	EventIDPrefix     = "evt_"
	SituationIDPrefix = "sit_"
	EpisodeIDPrefix   = "epi_"
	TriggerIDPrefix   = "trg_"
	DecisionIDPrefix  = "dec_"
	IntentIDPrefix    = "int_"
	CommandIDPrefix   = "cmd_"
	ArtifactIDPrefix  = "art_"
	ReplayIDPrefix    = "rpl_"
)

// ValidateID returns an error if id does not start with the expected prefix.
func ValidateID(id string, prefix string) error {
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return fmt.Errorf("id %q does not have required prefix %q", id, prefix)
	}
	return nil
}

// TenantID is the default tenant when none is supplied.
const TenantID = "default"

// PartitionCount is the default number of virtual partitions.
const PartitionCount = 64
