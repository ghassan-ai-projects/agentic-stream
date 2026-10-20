package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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
	ErrBootChanged     = errors.New("plantwire: the device rebooted since the command was bound")
)

type CommandOrder struct {
	CommandID      string
	IdempotencyKey string
	PolicyDigest   string
	Device         string
	Command        string
	Args           map[string]any
	ExpectedBootID string
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

func (s Settlement) Verifiable() bool {
	return s.Phase == SettlementSucceeded || s.Phase == SettlementUnknown
}

type inFlight struct {
	order CommandOrder
	boot  string
	phase string
}

func (c *Conversation) BootOf(device string) (string, error) {
	if !c.accepted {
		return "", ErrNoSession
	}
	born := c.ingest.devices[device]
	if born == nil {
		return "", fmt.Errorf("%w: %s", ErrDeviceNotBorn, device)
	}
	return born.boot, nil
}

func (c *Conversation) Prepare(o CommandOrder) (map[string]any, error) {
	boot, err := c.BootOf(o.Device)
	if err != nil {
		return nil, err
	}
	if boot != o.ExpectedBootID {
		return nil, fmt.Errorf("%w: %s is on %s, the command expects %s", ErrBootChanged, o.Device, boot, o.ExpectedBootID)
	}
	if _, twice := c.commands[o.CommandID]; twice {
		return nil, fmt.Errorf("%w: %s", ErrCommandInFlight, o.CommandID)
	}
	return c.out.stamp(commandRecord(o, c.epoch)), nil
}

func (c *Conversation) Register(o CommandOrder) {
	c.commands[o.CommandID] = &inFlight{order: o, boot: o.ExpectedBootID, phase: "sent"}
}

// Discard gives back the sequence number of a record that was never written,
// so a record that cannot be encoded leaves no gap in the stream.
func (c *Conversation) Discard(record map[string]any) {
	seq, ok := record["seq"].(uint64)
	if ok && seq+1 == c.out.next {
		c.out.next = seq
	}
}

func (c *Conversation) Forget(commandID string) {
	delete(c.commands, commandID)
}

func CommandDigest(o CommandOrder) (string, error) {
	content := map[string]any{"device": o.Device, "command": o.Command, "args": o.Args, "expected_boot_id": o.ExpectedBootID}
	canonical, err := canonicaljson.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("plantwire: command %s has no canonical content: %w", o.CommandID, err)
	}
	return canonicaljson.ContentDigest(canonical), nil
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

func commandRecord(o CommandOrder, epoch int64) Record {
	return Record{Type: TypeCommand, Fields: map[string]any{"command_id": o.CommandID, "idempotency_key": o.IdempotencyKey,
		"device": o.Device, "command": o.Command, "args": o.Args, "expected_boot_id": o.ExpectedBootID, "deadline_ns": o.DeadlineNS,
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
	if settlement.Verifiable() {
		c.watch(command.order, r)
	}
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
