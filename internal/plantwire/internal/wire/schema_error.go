package wire

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	schemakind "github.com/santhosh-tekuri/jsonschema/v6/kind"
)

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

type SchemaError struct {
	Schema  string
	Members []string
	cause   error
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("plantwire: %s fails its schema at %q", e.Schema, e.Members)
}

func (e *SchemaError) Unwrap() error { return e.cause }

func validateAgainst(name string, schema *jsonschema.Schema, document any) error {
	err := schema.Validate(document)
	var invalid *jsonschema.ValidationError
	if errors.As(err, &invalid) {
		return &SchemaError{Schema: name, Members: failedMembers(invalid), cause: err}
	}
	if err != nil {
		return fmt.Errorf("plantwire: validate %s: %w", name, err)
	}
	return nil
}

func failedMembers(root *jsonschema.ValidationError) []string {
	var members []string
	for _, leaf := range leavesOf(root) {
		members = append(members, membersOf(leaf)...)
	}
	slices.Sort(members)
	return slices.Compact(members)
}

func leavesOf(failure *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(failure.Causes) == 0 {
		return []*jsonschema.ValidationError{failure}
	}
	var leaves []*jsonschema.ValidationError
	for _, cause := range failure.Causes {
		leaves = append(leaves, leavesOf(cause)...)
	}
	return leaves
}

func membersOf(leaf *jsonschema.ValidationError) []string {
	at := pointer(leaf.InstanceLocation)
	switch failed := leaf.ErrorKind.(type) {
	case *schemakind.Required:
		return below(at, failed.Missing)
	case *schemakind.AdditionalProperties:
		return below(at, failed.Properties)
	}
	return []string{at}
}

func below(at string, names []string) []string {
	members := make([]string, len(names))
	for i, name := range names {
		members[i] = at + clip(pointer([]string{name}))
	}
	return members
}

func pointer(location []string) string {
	var path strings.Builder
	for _, token := range location {
		path.WriteString("/" + pointerEscaper.Replace(token))
	}
	return path.String()
}
