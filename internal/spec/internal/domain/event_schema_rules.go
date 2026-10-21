package domain

import (
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const (
	eventSchemaURN    = "urn:agentic-stream:event-schema:"
	allowedFieldTypes = "number, integer, string, boolean or empty (number)"
)

var (
	eventFieldTypes = []string{"", "number", "integer", "string", "boolean"}
	plainFieldName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (e registryEntry) validate(ref string) error {
	if err := e.validateName(ref); err != nil {
		return err
	}
	if err := e.validateFields(ref); err != nil {
		return err
	}
	return compileEventSchema(e.definition(ref))
}

func (e registryEntry) validateName(ref string) error {
	if e.EventType == "" || e.SchemaVersion == "" {
		return fmt.Errorf("event schema %q must declare event_type and schema_version", ref)
	}
	if ref != e.EventType+"/"+e.SchemaVersion {
		return fmt.Errorf("event schema %q must be named %q", ref, e.EventType+"/"+e.SchemaVersion)
	}
	return nil
}

func (e registryEntry) validateFields(ref string) error {
	for _, name := range slices.Sorted(maps.Keys(e.Fields)) {
		if err := e.Fields[name].validate(name); err != nil {
			return fmt.Errorf("event schema %q field %q: %w", ref, name, err)
		}
	}
	return nil
}

func (f registryField) validate(name string) error {
	if !plainFieldName.MatchString(name) {
		return fmt.Errorf("the field name is not a plain name (a letter or underscore, then letters, digits or underscores)")
	}
	if !plainFieldName.MatchString(f.Path) {
		return fmt.Errorf("path %q is not a plain field name", f.Path)
	}
	if !slices.Contains(eventFieldTypes, f.Type) {
		return fmt.Errorf("type %q is not one of %s", f.Type, allowedFieldTypes)
	}
	return f.validateEnum()
}

func (f registryField) validateEnum() error {
	if f.Enum == nil {
		return nil
	}
	if f.Type != "string" {
		return fmt.Errorf("enum is allowed only on a string field")
	}
	if len(f.Enum) == 0 {
		return fmt.Errorf("enum must list at least one value")
	}
	return nil
}

func compileEventSchema(definition EventSchema) error {
	document, err := EventSchemaJSON(definition)
	if err != nil {
		return err
	}
	if _, err := canonicaljson.CompileSchemaJSON(eventSchemaURN+definition.Ref, document); err != nil {
		return fmt.Errorf("event schema %q does not compile to a JSON Schema: %w", definition.Ref, err)
	}
	return nil
}
