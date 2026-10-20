package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

const (
	TypeCommand = "DCMD"
	TypeReceipt = "DRECEIPT"
	TypeResult  = "DRESULT"

	fallbackSafeOff = "safe_off"
	reasonEnding    = "session_ending"
)

const (
	SettlementQueued    = "queued"
	SettlementAccepted  = "accepted"
	SettlementSucceeded = "succeeded"
	SettlementRefused   = "refused"
	SettlementFailed    = "failed"
	SettlementUnknown   = "unknown"
	SettlementNotSent   = "not_sent"
)

var (
	ErrNoSession       = errors.New("plantwire: no node session is live")
	ErrDeviceNotBorn   = errors.New("plantwire: the device is not born in the live session")
	ErrCommandInFlight = errors.New("plantwire: the command is already in flight")
)

type CommandOrder struct {
	CommandID      string
	IdempotencyKey string
	PolicyDigest   string
	Device         string
	Command        string
	Args           map[string]any
	DeadlineNS     int64
}

type Receipt struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	BootID    string `json:"boot_id"`
}

type Result struct {
	CommandID       string `json:"command_id"`
	ReportedOutcome string `json:"reported_outcome"`
	BootID          string `json:"boot_id"`
}

type Settlement struct {
	CommandID string
	Phase     string
	Reason    string
	Answer    map[string]any
	Cause     error
}

func (s Settlement) Terminal() bool {
	return s.Phase != SettlementQueued && s.Phase != SettlementAccepted
}

type inFlight struct {
	boot  string
	phase string
}

func (c *Conversation) Command(o CommandOrder) (map[string]any, error) {
	if !c.accepted {
		return nil, ErrNoSession
	}
	device := c.ingest.devices[o.Device]
	if device == nil {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotBorn, o.Device)
	}
	if _, twice := c.commands[o.CommandID]; twice {
		return nil, fmt.Errorf("%w: %s", ErrCommandInFlight, o.CommandID)
	}
	c.commands[o.CommandID] = &inFlight{boot: device.boot, phase: "sent"}
	return c.out.stamp(commandRecord(o, device.boot, c.epoch)), nil
}

func (c *Conversation) Abandon() []Settlement {
	settlements := make([]Settlement, 0, len(c.commands))
	for _, id := range slices.Sorted(maps.Keys(c.commands)) {
		settlements = append(settlements, abandoned(id, c.commands[id]))
	}
	clear(c.commands)
	return settlements
}

func abandoned(id string, command *inFlight) Settlement {
	if command.phase == SettlementQueued {
		return Settlement{CommandID: id, Phase: SettlementRefused, Reason: reasonEnding}
	}
	return Settlement{CommandID: id, Phase: SettlementUnknown, Reason: "the session ended before the command's terminal answer"}
}

func commandRecord(o CommandOrder, boot string, epoch int64) Record {
	return Record{Type: TypeCommand, Fields: map[string]any{"command_id": o.CommandID, "idempotency_key": o.IdempotencyKey,
		"device": o.Device, "command": o.Command, "args": o.Args, "expected_boot_id": boot, "deadline_ns": o.DeadlineNS,
		"not_before_ns": 0, "authority_epoch": epoch, "policy_digest": o.PolicyDigest, "fallback_on_lease_loss": fallbackSafeOff}}
}

func (c *Conversation) receipt(rc Receipt, r *Reaction) {
	command := c.commands[rc.CommandID]
	if command == nil || command.phase == SettlementAccepted {
		r.Dropped = "a receipt for no command awaiting one"
		return
	}
	settlement := Settlement{CommandID: rc.CommandID, Phase: rc.Status, Reason: rc.Reason, Answer: receiptAnswer(rc)}
	if rc.Status == SettlementAccepted && rc.BootID != command.boot {
		settlement.Phase, settlement.Reason = SettlementUnknown, "accepted on another boot than the command expected"
	}
	c.settle(command, settlement, r)
}

func (c *Conversation) result(rs Result, r *Reaction) {
	command := c.commands[rs.CommandID]
	if command == nil || command.phase != SettlementAccepted {
		r.Dropped = "a result for no accepted command"
		return
	}
	settlement := Settlement{CommandID: rs.CommandID, Phase: rs.ReportedOutcome, Reason: "the device reported " + rs.ReportedOutcome,
		Answer: resultAnswer(rs)}
	if rs.BootID != command.boot {
		settlement.Phase, settlement.Reason = SettlementUnknown, "a result from another boot than the command expected"
	}
	c.settle(command, settlement, r)
}

func (c *Conversation) settle(command *inFlight, settlement Settlement, r *Reaction) {
	command.phase = settlement.Phase
	if settlement.Terminal() {
		delete(c.commands, settlement.CommandID)
	}
	r.Settlements = append(r.Settlements, settlement)
}

func receiptAnswer(rc Receipt) map[string]any {
	answer := map[string]any{"type": TypeReceipt, "status": rc.Status, "boot_id": rc.BootID}
	if rc.Reason != "" {
		answer["reason"] = rc.Reason
	}
	return answer
}

func resultAnswer(rs Result) map[string]any {
	return map[string]any{"type": TypeResult, "reported_outcome": rs.ReportedOutcome, "boot_id": rs.BootID}
}
