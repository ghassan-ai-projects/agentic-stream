package episodes

import (
	"strings"
	"testing"
)

func TestPersistedRequestValidatesTraceBeforeBudget(t *testing.T) {
	t.Parallel()
	req := &Request{RequestJSON: []byte(`{"traceparent":"invalid","budget":{"wall_time":"invalid"},"snapshot":{"entity":{"id":5}}}`)}
	err := hydratePersistedRequest(req)
	if err == nil || !strings.HasPrefix(err.Error(), "validate persisted request trace context:") {
		t.Fatalf("error precedence: %v", err)
	}
	if req.wallTimeValidated || req.EntityID != "" {
		t.Fatalf("request populated after invalid trace: %#v", req)
	}
}
