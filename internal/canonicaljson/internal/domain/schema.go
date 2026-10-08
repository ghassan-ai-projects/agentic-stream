package domain

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func CompileSchema(id string, document any) (*jsonschema.Schema, error) {
	compiler := offlineCompiler()
	if err := compiler.AddResource(id, document); err != nil {
		return nil, fmt.Errorf("add schema resource %s: %w", id, err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", id, err)
	}
	return schema, nil
}

func CompileSchemaJSON(id string, data []byte) (*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode schema %s: %w", id, err)
	}
	return CompileSchema(id, document)
}

func offlineCompiler() *jsonschema.Compiler {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})
	return compiler
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
}
