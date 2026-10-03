package spec

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// rawSpec mirrors the external YAML/JSON shape exactly.
type rawSpec struct {
	APIVersion string     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string     `yaml:"kind" json:"kind"`
	Metadata   Metadata   `yaml:"metadata" json:"metadata"`
	Inputs     []Input    `yaml:"inputs" json:"inputs"`
	Time       TimePolicy `yaml:"time" json:"time"`
	Windows    []Window   `yaml:"windows" json:"windows"`
	Operators  []Operator `yaml:"operators" json:"operators"`
	Situation  Situation  `yaml:"situation" json:"situation"`
	Cognition  Cognition  `yaml:"cognition" json:"cognition"`
	Actions    Actions    `yaml:"actions" json:"actions"`
}

// parseRawSpec decodes the YAML strictly: duplicate keys and unknown fields
// are rejected, and the document must be an agentic-stream/v1 SituationSpec.
func parseRawSpec(data []byte) (*rawSpec, error) {
	if err := checkDocumentKeys(data); err != nil {
		return nil, err
	}
	raw, err := decodeRawSpec(data)
	if err != nil {
		return nil, err
	}
	if err := checkSpecIdentity(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func checkDocumentKeys(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	return checkDuplicateKeys(&root, "")
}

// checkDuplicateKeys walks a YAML document and rejects duplicate mapping keys.
func checkDuplicateKeys(n *yaml.Node, path string) error {
	if n.Kind == yaml.MappingNode {
		return checkMappingKeys(n, path)
	}
	for _, child := range n.Content {
		if err := checkDuplicateKeys(child, path); err != nil {
			return err
		}
	}
	return nil
}

// checkMappingKeys rejects a repeated scalar key in one mapping, then checks
// each value under its key path.
func checkMappingKeys(n *yaml.Node, path string) error {
	seen := make(map[string]struct{}, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		key := keyNode.Value
		if _, ok := seen[key]; ok {
			return &CompileError{Path: path, Message: fmt.Sprintf("duplicate key %q", key)}
		}
		seen[key] = struct{}{}
		if i+1 >= len(n.Content) {
			continue
		}
		if err := checkDuplicateKeys(n.Content[i+1], joinKeyPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func joinKeyPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func decodeRawSpec(data []byte) (*rawSpec, error) {
	var raw rawSpec
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}
	return &raw, nil
}

func checkSpecIdentity(raw *rawSpec) error {
	if raw.APIVersion != "agentic-stream/v1" {
		return &CompileError{Path: "apiVersion", Message: fmt.Sprintf("expected agentic-stream/v1, got %q", raw.APIVersion)}
	}
	if raw.Kind != "SituationSpec" {
		return &CompileError{Path: "kind", Message: fmt.Sprintf("expected SituationSpec, got %q", raw.Kind)}
	}
	return nil
}
