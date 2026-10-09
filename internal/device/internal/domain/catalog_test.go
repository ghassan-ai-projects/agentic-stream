package domain_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

const (
	validRoute = `"operation": "o", "target": "t", "selector_field": "s", "expires_after_ms": 1, "presets": {"p": {"x": 1}}`
	routeKey   = `"r"`
)

func catalogOf(name, routeBody, rest string) string {
	return `{"protocol_version": 1, ` + rest + `"routes": {` + name + `: {` + routeBody + `}}}`
}

func TestLoadCapabilityCatalogRejectsWhatItCannotFullyEnforce(t *testing.T) {
	t.Parallel()
	route := func(replace, with string) string { return strings.Replace(validRoute, replace, with, 1) }
	cases := map[string]struct {
		document string
		want     string
	}{
		"no routes":                        {`{"protocol_version": 1, "routes": {}}`, "has no routes"},
		"protocol zero":                    {strings.Replace(catalogOf(routeKey, validRoute, ""), `"protocol_version": 1`, `"protocol_version": 0`, 1), "protocol_version 0 is unsupported"},
		"future protocol":                  {strings.Replace(catalogOf(routeKey, validRoute, ""), `"protocol_version": 1`, `"protocol_version": 2`, 1), "protocol_version 2 is unsupported"},
		"empty route name":                 {catalogOf(`""`, validRoute, ""), "empty route"},
		"unknown field":                    {`{"protocol_version": 1, "gremlin": true, "routes": {}}`, "decode capability catalog"},
		"not JSON":                         {`{`, "decode capability catalog"},
		"trailing JSON":                    {catalogOf(routeKey, validRoute, "") + ` {}`, "contains trailing JSON"},
		"trailing garbage":                 {catalogOf(routeKey, validRoute, "") + ` }`, "decode trailing capability catalog data"},
		"route without operation":          {catalogOf(routeKey, route(`"operation": "o", `, ""), ""), "must set operation, target, and selector_field"},
		"route without selector":           {catalogOf(routeKey, route(`"selector_field": "s", `, ""), ""), "must set operation, target, and selector_field"},
		"route never expires":              {catalogOf(routeKey, route(`"expires_after_ms": 1`, `"expires_after_ms": 0`), ""), "positive expires_after_ms"},
		"route without presets":            {catalogOf(routeKey, route(`"presets": {"p": {"x": 1}}`, `"presets": {}`), ""), "has no presets"},
		"bound without preset":             {catalogOf(routeKey, validRoute+`, "bounds": {"y": {"max": 1}}`, ""), `does not produce bounded parameter "y"`},
		"bound missing from one preset":    {catalogOf(routeKey, route(`"p": {"x": 1}`, `"p1": {"x": 1}, "p2": {}`)+`, "bounds": {"x": {"max": 1}}`, ""), "does not produce bounded parameter"},
		"empty bounds parameter":           {catalogOf(routeKey, validRoute+`, "bounds": {"": {"max": 1}}`, ""), "empty bounds parameter"},
		"inverted bound":                   {catalogOf(routeKey, validRoute+`, "bounds": {"x": {"min": 2, "max": 1}}`, ""), "minimum 2 above maximum 1"},
		"non-finite maximum":               {catalogOf(routeKey, validRoute+`, "bounds": {"x": {"max": 1e400}}`, ""), "decode capability catalog"},
		"binding to another target":        {catalogOf(routeKey, validRoute+`, "target_bindings": {"zone": "other"}`, ""), "want route target"},
		"empty binding":                    {catalogOf(routeKey, validRoute+`, "target_bindings": {"zone": ""}`, ""), "empty target binding"},
		"safe stop with another operation": {catalogOf(routeKey, validRoute, `"safe_stops": {"t": {"operation": "set_led", "expires_after_ms": 1}}, `), "must use the catalog operation"},
		"safe stop that never expires":     {catalogOf(routeKey, validRoute, `"safe_stops": {"t": {"operation": "safe_stop", "expires_after_ms": 0}}, `), "expires_after_ms between 1 and 86400000"},
		"safe stop that expires too late":  {catalogOf(routeKey, validRoute, `"safe_stops": {"t": {"operation": "safe_stop", "expires_after_ms": 86400001}}, `), "expires_after_ms between 1 and 86400000"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			catalog, err := domain.LoadCapabilityCatalog([]byte(tc.document))
			assertRefusal(t, err, tc.want)
			if catalog != nil {
				t.Fatalf("a rejected catalog was returned: %+v", catalog)
			}
		})
	}
}

func TestCatalogDigestRefusesAMissingOrInvalidCatalog(t *testing.T) {
	t.Parallel()
	var missing *domain.CapabilityCatalog
	_, missingErr := missing.Digest()
	_, invalidErr := (&domain.CapabilityCatalog{ProtocolVersion: 1}).Digest()
	assertRefusal(t, missingErr, "capability catalog is required")
	assertRefusal(t, invalidErr, "validate capability catalog")
}
