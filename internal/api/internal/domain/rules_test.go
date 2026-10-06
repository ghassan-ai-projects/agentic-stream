package domain

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseCursorAcceptsEmptyAndNonNegativeIntegers(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]int64{"": 0, "0": 0, "42": 42} {
		if got, err := ParseCursor(raw); err != nil || got != want {
			t.Errorf("ParseCursor(%q) = %d, %v", raw, got, err)
		}
	}
	for _, bad := range []string{"-1", "x", "1.5"} {
		if _, err := ParseCursor(bad); err == nil {
			t.Errorf("ParseCursor(%q) accepted", bad)
		}
	}
}

func TestStreamDefaultsAndPageBounds(t *testing.T) {
	t.Parallel()
	timing := StreamTiming{IdleInterval: time.Second}.WithDefaults()
	if timing.PollInterval != 500*time.Millisecond || timing.IdleInterval != time.Second || timing.RetryAfter != 2*time.Second {
		t.Fatalf("timing = %+v", timing)
	}
	for in, want := range map[int]int{0: 100, -5: 100, 1001: 100, 7: 7, 1000: 1000} {
		if got := NormalizePageSize(in); got != want {
			t.Errorf("NormalizePageSize(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestDedupSuppressesRepeatsWithinABoundedWindow(t *testing.T) {
	t.Parallel()
	d := NewDedup(1)
	if d.Repeated("s", "1") || !d.Repeated("s", "1") {
		t.Fatal("first sight must pass and the repeat must be suppressed")
	}
	for _, id := range []string{"2", "3", "4", "5"} {
		d.Repeated("s", id)
	}
	if d.Repeated("s", "1") {
		t.Fatal("window did not slide: an old event was still remembered")
	}
}

func TestEventTypeAllowListAndFrames(t *testing.T) {
	t.Parallel()
	if !AllowedEventType(nil, "x") || !AllowedEventType(map[string]struct{}{"x": {}}, "x") || AllowedEventType(map[string]struct{}{"y": {}}, "x") {
		t.Fatal("allow-list rule is wrong")
	}
	frame, err := EventFrame(7, "t", 2*time.Second, map[string]any{"b": 1, "a": 2})
	if err != nil || frame != "id: 7\nevent: t\nretry: 2000\ndata: {\"a\":2,\"b\":1}\n\n" {
		t.Fatalf("event frame = %q err %v", frame, err)
	}
	control, err := ControlFrame("stream_error", map[string]any{"code": "x"})
	if err != nil || control != "event: stream_error\ndata: {\"code\":\"x\"}\n\n" {
		t.Fatalf("control frame = %q err %v", control, err)
	}
	if CommentFrame("idle") != ": idle\n\n" {
		t.Fatal("comment frame is wrong")
	}
}

func TestProblemForMapsEveryFailureClass(t *testing.T) {
	t.Parallel()
	want := map[StreamFailure]int{FailureCursorExpired: 409, FailureSubscriberTooSlow: 429, FailureNotificationPoison: 503, FailureOther: 500}
	for failure, status := range want {
		if got := ProblemFor(failure); got.Status != status || got.Code == "" || got.Detail == "" {
			t.Errorf("ProblemFor(%d) = %+v", failure, got)
		}
	}
}

func TestBearerMatchingIsExactOrTrimmed(t *testing.T) {
	t.Parallel()
	if !ExactBearer("Bearer tok", "tok") || ExactBearer("Bearer  tok", "tok") || ExactBearer("tok", "tok") {
		t.Fatal("exact bearer rule is wrong")
	}
	if !LooseBearer("Bearer  tok ", " tok ") || LooseBearer("Bearer other", "tok") || LooseBearer("Bearer ", "") {
		t.Fatal("loose bearer rule is wrong")
	}
}

func TestDecodeApprovalInputRequiresOneCompleteDocument(t *testing.T) {
	t.Parallel()
	signature := strings.Repeat("A", 86) + "=="
	valid := `{"approver_id":"a","approved":true,"signature":"` + signature + `","reason":"ok"}`
	if _, err := DecodeApprovalInput(bytes.NewBufferString(valid)); err != nil {
		t.Fatalf("valid input: %v", err)
	}
	for name, body := range map[string]string{
		"unknown field": `{"approver_id":"a","approved":true,"signature":"` + signature + `","reason":"ok","x":1}`,
		"missing":       `{"approver_id":"a"}`,
		"two documents": valid + valid,
		"not json":      "nope",
	} {
		if _, err := DecodeApprovalInput(bytes.NewBufferString(body)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if (ApprovalConfig{}).Configured() {
		t.Fatal("empty config reported configured")
	}
	_ = errors.New
}
