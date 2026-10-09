package domain

import (
	"testing"
	"time"
)

func TestParseCursor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw     string
		want    int64
		wantErr bool
	}{
		{raw: "", want: 0},
		{raw: "0", want: 0},
		{raw: "42", want: 42},
		{raw: "9223372036854775807", want: 9223372036854775807},
		{raw: "-1", wantErr: true},
		{raw: "x", wantErr: true},
		{raw: "1.5", wantErr: true},
		{raw: "9223372036854775808", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("cursor "+tt.raw, func(t *testing.T) {
			t.Parallel()
			got, err := ParseCursor(tt.raw)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("ParseCursor(%q) = %d, %v; want %d, error=%v", tt.raw, got, err, tt.want, tt.wantErr)
			}
			if tt.wantErr && err.Error() != "cursor must be a non-negative integer" {
				t.Fatalf("ParseCursor(%q) error = %q", tt.raw, err)
			}
		})
	}
}

func TestStreamTimingReplacesNonPositiveIntervalsWithDefaults(t *testing.T) {
	t.Parallel()
	custom := StreamTiming{PollInterval: time.Second, IdleInterval: 2 * time.Second, RetryAfter: 3 * time.Second}
	tests := []struct {
		name string
		in   StreamTiming
		want StreamTiming
	}{
		{"unset", StreamTiming{}, StreamTiming{PollInterval: 500 * time.Millisecond, IdleInterval: 15 * time.Second, RetryAfter: 2 * time.Second}},
		{"negative", StreamTiming{PollInterval: -1, IdleInterval: -1, RetryAfter: -1}, StreamTiming{PollInterval: 500 * time.Millisecond, IdleInterval: 15 * time.Second, RetryAfter: 2 * time.Second}},
		{"partly set", StreamTiming{IdleInterval: time.Second}, StreamTiming{PollInterval: 500 * time.Millisecond, IdleInterval: time.Second, RetryAfter: 2 * time.Second}},
		{"all set", custom, custom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.WithDefaults(); got != tt.want {
				t.Fatalf("WithDefaults() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizePageSizeKeepsOnlyPositiveSizesUpToTheLimit(t *testing.T) {
	t.Parallel()
	for in, want := range map[int]int{0: 100, -5: 100, 1: 1, 7: 7, 1000: 1000, 1001: 100} {
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
	if d.Repeated("other", "1") {
		t.Fatal("the same id from another source is a different event")
	}
	for _, id := range []string{"2", "3", "4", "5"} {
		d.Repeated("s", id)
	}
	if d.Repeated("s", "1") {
		t.Fatal("window did not slide: an old event was still remembered")
	}
}

func TestAllowedEventTypeTreatsAnEmptyAllowListAsOpen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		allowed map[string]struct{}
		want    bool
	}{
		{"nil allow-list admits everything", nil, true},
		{"empty allow-list admits everything", map[string]struct{}{}, true},
		{"listed type", map[string]struct{}{"x": {}}, true},
		{"unlisted type", map[string]struct{}{"y": {}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := AllowedEventType(tt.allowed, "x"); got != tt.want {
				t.Fatalf("AllowedEventType = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFramesFollowTheServerSentEventsFormat(t *testing.T) {
	t.Parallel()
	t.Run("event frame carries cursor, type, retry and canonical data", func(t *testing.T) {
		t.Parallel()
		frame, err := EventFrame(7, "t", 2*time.Second, map[string]any{"b": 1, "a": 2})
		if want := "id: 7\nevent: t\nretry: 2000\ndata: {\"a\":2,\"b\":1}\n\n"; err != nil || frame != want {
			t.Fatalf("event frame = %q, %v; want %q", frame, err, want)
		}
	})
	t.Run("control frame has no id so it cannot move the resume cursor", func(t *testing.T) {
		t.Parallel()
		frame, err := ControlFrame("stream_error", map[string]any{"code": "x"})
		if want := "event: stream_error\ndata: {\"code\":\"x\"}\n\n"; err != nil || frame != want {
			t.Fatalf("control frame = %q, %v; want %q", frame, err, want)
		}
	})
	t.Run("comment frame", func(t *testing.T) {
		t.Parallel()
		if got := CommentFrame("idle"); got != ": idle\n\n" {
			t.Fatalf("comment frame = %q", got)
		}
	})
	t.Run("unencodable data is refused", func(t *testing.T) {
		t.Parallel()
		if _, err := EventFrame(1, "t", time.Second, map[string]any{"c": make(chan int)}); err == nil {
			t.Fatal("event with a channel value framed")
		}
		if _, err := ControlFrame("t", map[string]any{"c": make(chan int)}); err == nil {
			t.Fatal("control event with a channel value framed")
		}
	})
}

func TestProblemForMapsEveryFailureClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		failure    StreamFailure
		wantStatus int
		wantCode   string
	}{
		{FailureCursorExpired, 409, "cursor_expired"},
		{FailureSubscriberTooSlow, 429, "subscriber_too_slow"},
		{FailureNotificationPoison, 503, "notification_retry"},
		{FailureOther, 500, "notification_stream_failed"},
		{StreamFailure(99), 500, "notification_stream_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.wantCode, func(t *testing.T) {
			t.Parallel()
			got := ProblemFor(tt.failure)
			if got.Status != tt.wantStatus || got.Code != tt.wantCode || got.Detail == "" {
				t.Fatalf("ProblemFor(%d) = %+v, want %d %s", tt.failure, got, tt.wantStatus, tt.wantCode)
			}
		})
	}
}
