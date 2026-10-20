package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"
)

type EventSchemas struct {
	entries map[string]registryEntry
	schemas map[string]EventSchema
}

var builtinEventSchemas = sync.OnceValues(decodeBuiltinEventSchemas)

func BuiltinEventSchemas() *EventSchemas {
	registry, err := builtinEventSchemas()
	if err != nil {
		panic(fmt.Sprintf("eventschema: corrupt embedded registry data: %v", err))
	}
	return registry
}

func decodeBuiltinEventSchemas() (*EventSchemas, error) {
	var entries map[string]registryEntry
	if err := json.Unmarshal(registryData, &entries); err != nil {
		return nil, fmt.Errorf("decode eventschema registry data: %w", err)
	}
	return newEventSchemas(entries), nil
}

func newEventSchemas(entries map[string]registryEntry) *EventSchemas {
	schemas := make(map[string]EventSchema, len(entries))
	for ref, entry := range entries {
		schemas[ref] = entry.definition(ref)
	}
	return &EventSchemas{entries: entries, schemas: schemas}
}

func (s *EventSchemas) Lookup(ref string) (EventSchema, bool) {
	if s == nil {
		return BuiltinEventSchemas().Lookup(ref)
	}
	definition, ok := s.schemas[ref]
	return definition, ok
}

func (s *EventSchemas) Merge(source string, document []byte) (*EventSchemas, error) {
	added, err := decodeEventSchemaDocument(source, document)
	if err != nil {
		return nil, err
	}
	merged := maps.Clone(s.entries)
	for _, ref := range slices.Sorted(maps.Keys(added)) {
		if err := mergeEventSchemaEntry(merged, ref, added[ref]); err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
		}
	}
	return newEventSchemas(merged), nil
}

func decodeEventSchemaDocument(source string, document []byte) (map[string]registryEntry, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var entries map[string]registryEntry
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode event schemas %s: %w", source, err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("decode event schemas %s: trailing data after the schema document", source)
	}
	return entries, nil
}

func mergeEventSchemaEntry(merged map[string]registryEntry, ref string, entry registryEntry) error {
	if err := entry.validate(ref); err != nil {
		return err
	}
	existing, present := merged[ref]
	if present && !sameEventSchemaEntry(existing, entry) {
		return fmt.Errorf("event schema %q is already registered with a different definition", ref)
	}
	merged[ref] = entry
	return nil
}

func (e registryEntry) validate(ref string) error {
	if e.EventType == "" || e.SchemaVersion == "" {
		return fmt.Errorf("event schema %q must declare event_type and schema_version", ref)
	}
	if ref != e.EventType+"/"+e.SchemaVersion {
		return fmt.Errorf("event schema %q must be named %q", ref, e.EventType+"/"+e.SchemaVersion)
	}
	return nil
}

func sameEventSchemaEntry(a, b registryEntry) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
