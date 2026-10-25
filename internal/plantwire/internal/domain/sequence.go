package domain

import "math"

type Order int

const (
	InOrder Order = iota
	Duplicate
	Gap
)

type Receiver struct{ next uint64 }

func (r *Receiver) Accept(seq uint64) Order {
	switch {
	case seq < r.next:
		return Duplicate
	case seq == math.MaxUint64:
		return Gap
	case seq == r.next:
		r.next++
		return InOrder
	}
	r.next = seq + 1
	return Gap
}
