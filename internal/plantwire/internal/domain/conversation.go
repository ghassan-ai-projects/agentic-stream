package domain

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const (
	EndRefused   = "refused"
	EndNodeDeath = "node death"
)

type Arrival struct {
	Envelope
	Sequenced   bool
	Invalid     error
	ReceivedAt  time.Time
	Birth       *Birth
	Death       *NodeDeath
	DeviceBirth *DeviceBirth
	DeviceData  *DeviceData
	DeviceDeath *DeviceDeath
	Sync        *Sync
	Receipt     *Receipt
	Result      *Result
}

type Reaction struct {
	Replies       []Record
	Events        []contractsv1.Envelope
	Settlements   []Settlement
	Verifications []Verification
	Verdict       *Verdict
	Quarantined   []string
	BornKinds     map[string]string
	Rebirth       bool
	Dropped       string
	End           string
}

type Conversation struct {
	admission Admission
	epoch     int64
	in        Receiver
	out       transmitter
	accepted  bool
	asked     bool
	bdSeq     uint64
	heartbeat uint64
	ingest    *ingest
	commands  map[string]*inFlight
	watches   map[string]*watch
	evidence  []map[string]any
}

func NewConversation(admission Admission, epoch int64, mapping Mapping) *Conversation {
	return &Conversation{admission: admission, epoch: epoch, ingest: newIngest(mapping), commands: map[string]*inFlight{},
		watches: map[string]*watch{}}
}

func (c *Conversation) Tally() Tally { return c.ingest.tally }

func (c *Conversation) Receive(a Arrival) Reaction {
	var reaction Reaction
	if !c.inSequence(a, &reaction) {
		return reaction
	}
	if reaction.Dropped = c.unusable(a); reaction.Dropped == "" {
		c.react(a, &reaction)
	}
	return reaction
}

func (c *Conversation) Transmit(records []Record) []map[string]any {
	stamped := make([]map[string]any, len(records))
	for i, record := range records {
		stamped[i] = c.out.stamp(record)
	}
	return stamped
}

func (c *Conversation) Beat() []map[string]any {
	if !c.accepted {
		return nil
	}
	c.heartbeat++
	return []map[string]any{c.out.stamp(stateRecord(c.epoch, c.heartbeat, ""))}
}

func (c *Conversation) Goodbye() []map[string]any {
	if !c.accepted {
		return nil
	}
	return []map[string]any{c.out.stamp(goodbyeRecord(c.epoch, c.heartbeat))}
}

func (c *Conversation) inSequence(a Arrival, r *Reaction) bool {
	if !a.Sequenced {
		r.Dropped = "unsequenced"
		return false
	}
	switch c.in.Accept(a.Seq) {
	case Duplicate:
		r.Dropped = "duplicate"
		return false
	case Gap:
		c.askRebirth(r)
	}
	return true
}

func (c *Conversation) askRebirth(r *Reaction) {
	if !c.accepted || c.asked {
		return
	}
	c.asked, r.Rebirth = true, true
	r.Replies = append(r.Replies, rebirthRecord())
}

func (c *Conversation) unusable(a Arrival) string {
	switch {
	case a.Invalid != nil:
		return "invalid: " + a.Invalid.Error()
	case !c.accepted && a.Type != TypeNodeBirth:
		return "before the handshake"
	case c.accepted && a.Within(c.out.session, c.out.version) != nil:
		return "outside the session"
	}
	return ""
}

func (c *Conversation) react(a Arrival, r *Reaction) {
	switch a.Type {
	case TypeNodeBirth:
		c.birth(*a.Birth, r)
	case TypeNodeDeath:
		c.death(*a.Death, r)
	case TypeSync:
		r.Replies = append(r.Replies, c.barrier(a.Seq))
	case TypeReceipt:
		c.receipt(*a.Receipt, r)
	case TypeResult:
		c.result(*a.Result, r)
	}
	c.ingest.observe(a, r)
	c.verify(a, r)
}

func (c *Conversation) birth(b Birth, r *Reaction) {
	c.asked = false
	if c.accepted {
		c.rebirth(b, r)
		return
	}
	c.handshake(b, r)
	if c.accepted {
		c.commission(b, r)
	}
}

func (c *Conversation) rebirth(b Birth, r *Reaction) {
	if b.BDSeq <= c.bdSeq {
		r.Dropped = "a birth that does not advance bd_seq"
		return
	}
	verdict := c.admission.Admit(b)
	r.Verdict = &verdict
	if !verdict.Accepted() {
		r.Replies = append(r.Replies, stateRecord(c.epoch, c.heartbeat, verdict.Refusal))
		r.End = EndRefused
		return
	}
	c.bdSeq = b.BDSeq
	c.commission(b, r)
}

func (c *Conversation) commission(b Birth, r *Reaction) {
	r.Quarantined = c.admission.Inventory.Quarantined(b)
	r.BornKinds = b.KindDigests()
	c.ingest.rebirth(b, r.Quarantined)
}

func (c *Conversation) handshake(b Birth, r *Reaction) {
	verdict := c.admission.Admit(b)
	r.Verdict = &verdict
	c.accepted = verdict.Accepted()
	c.out = transmitter{session: b.SessionID, version: ProtocolVersion}
	c.bdSeq = b.BDSeq
	r.Replies = append(r.Replies, stateRecord(c.epoch, 0, verdict.Refusal))
	if !c.accepted {
		r.End = EndRefused
	}
}

func (c *Conversation) death(d NodeDeath, r *Reaction) {
	if d.BDSeq < c.bdSeq {
		r.Dropped = "a node death of an older birth"
		return
	}
	r.End = EndNodeDeath
}
