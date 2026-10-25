package wire

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const siteSchemaID = "urn:agentic-stream:plant-wire-site:1"

//go:embed site.schema.json
var siteSchemaFile []byte

var siteSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return canonicaljson.CompileSchemaJSON(siteSchemaID, siteSchemaFile)
})

func ParseSiteFile(raw []byte) (domain.Site, error) {
	if err := checkSiteFile(raw); err != nil {
		return domain.Site{}, err
	}
	var site domain.Site
	if err := decodeExactly(raw, &site); err != nil {
		return domain.Site{}, fmt.Errorf("plantwire: site file: %w", err)
	}
	return site, nil
}

func checkSiteFile(raw []byte) error {
	fields, err := parseObject(raw)
	if err != nil {
		return fmt.Errorf("plantwire: site file: %w", err)
	}
	schema, err := siteSchema()
	if err != nil {
		return fmt.Errorf("plantwire: compile the site file schema: %w", err)
	}
	return validateAgainst("site file", schema, fields)
}
