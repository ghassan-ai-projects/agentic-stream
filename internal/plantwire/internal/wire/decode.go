package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const (
	MaxRecordBytes = 64 << 10
	MaxBirthBytes  = 1 << 20
)

var (
	ErrNotARecord = errors.New("plantwire: not one JSON object")
	ErrTooLarge   = errors.New("plantwire: record larger than its limit")
	ErrNotExact   = errors.New("plantwire: an integer member is not an exact 64-bit integer")
	ErrNotSent    = errors.New("plantwire: a host record fails the contract")
)

type Inbound struct {
	domain.Envelope
	Sequenced bool
	Fields    map[string]any
	Invalid   error
	line      []byte
}

func Decode(line []byte) (Inbound, error) {
	if len(line) > MaxBirthBytes {
		return Inbound{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, len(line))
	}
	fields, err := parseObject(line)
	if err != nil {
		return Inbound{}, err
	}
	in := Inbound{Fields: fields, line: line}
	in.Type, _ = fields["type"].(string)
	in.Seq, in.Sequenced = sequenceOf(fields["seq"])
	in.Invalid = in.check()
	return in, nil
}

func (in *Inbound) check() error {
	if err := withinLimit(in.Type, len(in.line)); err != nil {
		return err
	}
	schema, err := recordSchema(in.Type)
	if err != nil {
		return err
	}
	if err := validateAgainst(in.Type, schema, in.Fields); err != nil {
		return err
	}
	return decodeExactly(in.line, &in.Envelope)
}

func (in Inbound) Arrival() domain.Arrival {
	arrival := domain.Arrival{Envelope: in.Envelope, Sequenced: in.Sequenced, Invalid: in.Invalid}
	if arrival.Invalid == nil {
		arrival.Invalid = in.decodeBody(&arrival)
	}
	return arrival
}

func (in Inbound) decodeBody(arrival *domain.Arrival) error {
	switch in.Type {
	case domain.TypeNodeBirth:
		arrival.Birth = &domain.Birth{}
		return decodeExactly(in.line, arrival.Birth)
	case domain.TypeNodeDeath:
		arrival.Death = &domain.NodeDeath{}
		return decodeExactly(in.line, arrival.Death)
	}
	return nil
}

func withinLimit(recordType string, size int) error {
	births := recordType == domain.TypeNodeBirth || recordType == domain.TypeDeviceBirth
	if size > MaxRecordBytes && !births {
		return fmt.Errorf("%w: %q of %d bytes", ErrTooLarge, clip(recordType), size)
	}
	return nil
}

func parseObject(raw []byte) (map[string]any, error) {
	var fields map[string]any
	if err := decodeNumbers(raw, &fields); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotARecord, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("%w: null", ErrNotARecord)
	}
	return fields, nil
}

func decodeExactly(raw []byte, target any) error {
	if err := decodeNumbers(raw, target); err != nil {
		return fmt.Errorf("%w: %w", ErrNotExact, err)
	}
	return nil
}

func decodeNumbers(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode: trailing data after the record")
	}
	return nil
}

func sequenceOf(value any) (uint64, bool) {
	number, isNumber := value.(json.Number)
	if !isNumber {
		return 0, false
	}
	seq, err := strconv.ParseUint(number.String(), 10, 64)
	return seq, err == nil
}
