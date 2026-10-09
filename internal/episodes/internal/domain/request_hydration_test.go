package domain

import (
	"strings"
	"testing"
)

func TestPersistedRequestValidatesTraceBeforeBudget(t *testing.T) {
	t.Parallel()
	req := &Request{RequestJSON: []byte(`{"traceparent":"invalid","budget":{"wall_time":"invalid"},"snapshot":{"entity":{"id":5}}}`)}
	err := HydratePersistedRequest(req)
	if err == nil || !strings.HasPrefix(err.Error(), "validate persisted request trace context:") {
		t.Fatalf("error precedence: %v", err)
	}
	if req.wallTimeValidated || req.EntityID != "" {
		t.Fatalf("request populated after invalid trace: %#v", req)
	}
}

func TestHydrationPreservesBoundaryErrorPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, raw, prefix string }{
		{"invalid JSON", "{", "decode persisted request trace context:"},
		{"invalid budget", `{"budget":{"wall_time":"soon"},"snapshot":{"entity":{"id":5}}}`, "validate persisted episode budget:"},
		{"invalid entity", `{"budget":{"wall_time":"1s"},"snapshot":{"entity":{"id":5}}}`, "load persisted request entity:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := HydratePersistedRequest(&Request{RequestJSON: []byte(test.raw)}); err == nil || !strings.HasPrefix(err.Error(), test.prefix) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	req := &Request{RequestJSON: []byte(`{"budget":{"wall_time":"1s"},"snapshot":{"entity":{"id":"motor"}},"cancellation_key":"episode:epi","supersession_key":"situation:sit"}`)}
	if err := HydratePersistedRequest(req); err != nil {
		t.Fatal(err)
	}
	if req.EntityID != "motor" || req.CancellationKey != "episode:epi" || req.SupersessionKey != "situation:sit" || !req.wallTimeValidated {
		t.Fatalf("request=%#v", req)
	}
}

func TestHydrationOfARequestWithoutASnapshotLeavesTheEntityUnbound(t *testing.T) {
	t.Parallel()
	req := &Request{RequestJSON: []byte(`{"budget":{"wall_time":"1s"}}`)}
	if err := HydratePersistedRequest(req); err != nil {
		t.Fatal(err)
	}
	if req.EntityID != "" || !req.wallTimeValidated {
		t.Fatalf("request = %#v", req)
	}
}
