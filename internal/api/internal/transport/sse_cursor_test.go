package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func TestFilteredDuplicatePageStillAdvancesResumeCursor(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/events", nil)
	checks := 0
	cfg := SSEConfig{PageSize: 10, Authorize: func(*http.Request, string) bool { checks++; return false }}
	event := contractsv1.CloudEvent{Source: "source", ID: "same", Type: "denied"}
	page := notify.Page{Records: []notify.Record{{Cursor: 1, Event: event}, {Cursor: 2, Event: event}}, NextCursor: 5}
	cursor := int64(0)
	if err := writePage(w, w, r, cfg, page, &cursor, domain.NewDedup(100)); err != nil {
		t.Fatal(err)
	}
	if cursor != 5 || checks != 1 || w.Body.Len() != 0 {
		t.Fatalf("cursor=%d authorizations=%d body=%q", cursor, checks, w.Body.String())
	}
}
