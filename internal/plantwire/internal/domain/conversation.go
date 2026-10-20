package domain

const (
	EndRefused   = "refused"
	EndNodeDeath = "node death"
)

type Arrival struct {
	Envelope
	Sequenced bool
	Invalid   error
	Birth     *Birth
	Death     *NodeDeath
}

type Reaction struct {
	Replies     []map[string]any
	Verdict     *Verdict
	Quarantined []string
	Rebirth     bool
	Dropped     string
	End         string
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
}

func NewConversation(admission Admission, epoch int64) *Conversation {
	return &Conversation{admission: admission, epoch: epoch}
}

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
	r.Replies = append(r.Replies, c.out.stamp(rebirthRecord()))
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
		r.Replies = append(r.Replies, c.out.stamp(syncRecord(a.Seq)))
	}
}

func (c *Conversation) birth(b Birth, r *Reaction) {
	c.asked, c.bdSeq = false, b.BDSeq
	if !c.accepted {
		c.handshake(b, r)
	}
	if c.accepted {
		r.Quarantined = c.admission.Inventory.Quarantined(b)
	}
}

func (c *Conversation) handshake(b Birth, r *Reaction) {
	verdict := c.admission.Admit(b)
	r.Verdict = &verdict
	c.accepted = verdict.Accepted()
	c.out = transmitter{session: b.SessionID, version: ProtocolVersion}
	r.Replies = append(r.Replies, c.out.stamp(stateRecord(c.epoch, 0, verdict.Refusal)))
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
