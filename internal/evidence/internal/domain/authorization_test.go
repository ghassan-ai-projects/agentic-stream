package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestValidateEnvelopeRefusesIncompleteOrOversizedRequests(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	tests := []struct {
		name   string
		mutate func(*Envelope)
		want   ErrorKind
	}{
		{"protocol version", func(e *Envelope) { e.ProtocolVersion = "2.0" }, InvalidArgument},
		{"episode", func(e *Envelope) { e.EpisodeID = "" }, InvalidArgument},
		{"call", func(e *Envelope) { e.CallID = "" }, InvalidArgument},
		{"tool", func(e *Envelope) { e.ToolName = "" }, InvalidArgument},
		{"tenant", func(e *Envelope) { e.TenantID = "" }, InvalidArgument},
		{"situation", func(e *Envelope) { e.SituationID = "" }, InvalidArgument},
		{"entity", func(e *Envelope) { e.EntityID = "" }, InvalidArgument},
		{"attempt", func(e *Envelope) { e.AttemptID = "" }, InvalidArgument},
		{"fence", func(e *Envelope) { e.Fence = 0 }, InvalidArgument},
		{"row budget", func(e *Envelope) { e.MaxRows = 0 }, InvalidArgument},
		{"byte budget", func(e *Envelope) { e.MaxBytes = 0 }, InvalidArgument},
		{"empty arguments", func(e *Envelope) { e.ArgumentsJSON = nil }, ResourceExhausted},
		{"oversized arguments", func(e *Envelope) { e.ArgumentsJSON = make([]byte, MaxArgumentsBytes+1) }, ResourceExhausted},
		{"oversized token", func(e *Envelope) { e.CapabilityToken = make([]byte, MaxCapabilityTokenBytes+1) }, ResourceExhausted},
		{"oversized frame", func(e *Envelope) { e.EncodedSize = MaxArgumentsBytes + MaxCapabilityTokenBytes + 1 }, ResourceExhausted},
		{"identity before size", func(e *Envelope) {
			e.EpisodeID = ""
			e.ArgumentsJSON = make([]byte, MaxArgumentsBytes+1)
		}, InvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := envelopeFor(scope)
			test.mutate(&request)
			requireRefusal(t, ValidateEnvelope(request), test.want)
		})
	}
	t.Run("arguments at the limit", func(t *testing.T) {
		t.Parallel()
		request := envelopeFor(scope)
		request.ArgumentsJSON = make([]byte, MaxArgumentsBytes)
		request.EncodedSize = MaxArgumentsBytes + MaxCapabilityTokenBytes
		if err := ValidateEnvelope(request); err != nil {
			t.Fatalf("ValidateEnvelope: %v", err)
		}
	})
}

func TestAuthorizeScopeRefusesEveryDimensionOutsideTheCapability(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	tests := []struct {
		name   string
		mutate func(*Envelope)
		epoch  string
		want   ErrorKind
	}{
		{"episode", func(e *Envelope) { e.EpisodeID = "episode-2" }, "epoch", PermissionDenied},
		{"attempt", func(e *Envelope) { e.AttemptID = "attempt-2" }, "epoch", PermissionDenied},
		{"fence", func(e *Envelope) { e.Fence = 2 }, "epoch", PermissionDenied},
		{"tenant", func(e *Envelope) { e.TenantID = "tenant-2" }, "epoch", PermissionDenied},
		{"situation", func(e *Envelope) { e.SituationID = "situation-2" }, "epoch", PermissionDenied},
		{"situation version", func(e *Envelope) { e.SituationVersion = 2 }, "epoch", PermissionDenied},
		{"entity", func(e *Envelope) { e.EntityID = "motor-2" }, "epoch", PermissionDenied},
		{"ungranted tool", func(e *Envelope) { e.ToolName = "shell.exec" }, "epoch", PermissionDenied},
		{"trace", func(e *Envelope) { e.Traceparent = "other-trace" }, "epoch", PermissionDenied},
		{"trace state", func(e *Envelope) { e.Tracestate = "vendor=1" }, "epoch", PermissionDenied},
		{"runtime epoch", func(*Envelope) {}, "another-epoch", PermissionDenied},
		{"range starts before the capability", func(e *Envelope) { e.From.Value = scope.From.Add(-time.Nanosecond) }, "epoch", PermissionDenied},
		{"range ends after the capability", func(e *Envelope) { e.Until.Value = scope.Until.Add(time.Nanosecond) }, "epoch", PermissionDenied},
		{"inverted range", func(e *Envelope) { e.From.Value, e.Until.Value = e.Until.Value, e.From.Value }, "epoch", PermissionDenied},
		{"missing range start", func(e *Envelope) { e.From = Timestamp{} }, "epoch", PermissionDenied},
		{"missing range end", func(e *Envelope) { e.Until = Timestamp{} }, "epoch", PermissionDenied},
		{"unreadable range start", func(e *Envelope) { e.From.Valid = false }, "epoch", PermissionDenied},
		{"unreadable range end", func(e *Envelope) { e.Until.Valid = false }, "epoch", PermissionDenied},
		{"fence beyond the signed range", func(e *Envelope) { e.Fence = 1 << 63 }, "epoch", InvalidArgument},
		{"situation version beyond the signed range", func(e *Envelope) { e.SituationVersion = 1 << 63 }, "epoch", InvalidArgument},
		{"scope mismatch is reported before the range", func(e *Envelope) {
			e.AttemptID = "attempt-2"
			e.From = Timestamp{}
		}, "epoch", PermissionDenied},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := envelopeFor(scope)
			test.mutate(&request)
			requireRefusal(t, AuthorizeScope(request, scope, test.epoch), test.want)
		})
	}
	t.Run("the granted request", func(t *testing.T) {
		t.Parallel()
		if err := AuthorizeScope(envelopeFor(scope), scope, "epoch"); err != nil {
			t.Fatalf("AuthorizeScope: %v", err)
		}
	})
	t.Run("the exact granted range", func(t *testing.T) {
		t.Parallel()
		request := envelopeFor(scope)
		request.From.Value, request.Until.Value = scope.From, scope.Until
		if err := AuthorizeScope(request, scope, "epoch"); err != nil {
			t.Fatalf("AuthorizeScope: %v", err)
		}
	})
	t.Run("any epoch when none is configured", func(t *testing.T) {
		t.Parallel()
		if err := AuthorizeScope(envelopeFor(scope), scope, ""); err != nil {
			t.Fatalf("AuthorizeScope: %v", err)
		}
	})
}

func TestBindArgumentsRequiresTheEntityAgreedByScopeAndRequest(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	trace := contractsv1.TraceContext{Traceparent: scope.Traceparent}
	tests := []struct {
		name      string
		argument  string
		requested string
		wantErr   bool
	}{
		{"agreeing entity", "motor-1", "motor-1", false},
		{"argument names another entity", "motor-2", "motor-1", true},
		{"request names another entity", "motor-1", "motor-2", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := envelopeFor(scope)
			request.EntityID = test.requested
			call, err := BindArguments(request, scope, trace, EvidenceGetArguments{EntityID: test.argument})
			if test.wantErr {
				requireRefusal(t, err, PermissionDenied)
				return
			}
			if err != nil || call.EntityID != "motor-1" || call.Fence != scope.Fence || call.SituationVersion != scope.SituationVersion {
				t.Fatalf("call = %+v, err %v", call, err)
			}
		})
	}
}

func TestBindArgumentsNeverExpandsTheGrantedBudgets(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	tests := []struct {
		name                string
		requestRows         uint64
		requestBytes        uint64
		wantRows, wantBytes uint64
	}{
		{"request below the grant", 3, 40, 3, 40},
		{"request above the grant", 50, 500, 10, 100},
		{"request equal to the grant", 10, 100, 10, 100},
		{"request without a budget", 0, 0, 10, 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := envelopeFor(scope)
			request.MaxRows, request.MaxBytes = test.requestRows, test.requestBytes
			call, err := BindArguments(request, scope, contractsv1.TraceContext{}, EvidenceGetArguments{EntityID: "motor-1"})
			if err != nil || call.MaxRows != test.wantRows || call.MaxBytes != test.wantBytes {
				t.Fatalf("budgets = %d rows %d bytes, want %d/%d (err %v)", call.MaxRows, call.MaxBytes, test.wantRows, test.wantBytes, err)
			}
		})
	}
}

func TestBindArgumentsCarriesTheRequestedTimeRange(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	request := envelopeFor(scope)
	request.From.Value = scope.From.Add(time.Minute)
	request.Until.Value = scope.Until.Add(-time.Minute)
	call, err := BindArguments(request, scope, contractsv1.TraceContext{}, EvidenceGetArguments{EntityID: "motor-1"})
	if err != nil || !call.From.Equal(request.From.Value) || !call.Until.Equal(request.Until.Value) {
		t.Fatalf("call range = %v..%v, want %v..%v (err %v)", call.From, call.Until, request.From.Value, request.Until.Value, err)
	}
}

func TestCallDeadlineIsCappedAtCapabilityExpiry(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	now := scope.IssuedAt
	tests := []struct {
		name     string
		deadline Timestamp
		want     time.Time
		wantKind ErrorKind
	}{
		{"absent deadline defaults to a minute", Timestamp{}, now.Add(time.Minute).Truncate(0), ""},
		{"deadline before expiry", Timestamp{Present: true, Valid: true, Value: now.Add(time.Second)}, now.Add(time.Second), ""},
		{"deadline beyond expiry", Timestamp{Present: true, Valid: true, Value: scope.ExpiresAt.Add(time.Hour)}, scope.ExpiresAt, ""},
		{"deadline at expiry", Timestamp{Present: true, Valid: true, Value: scope.ExpiresAt}, scope.ExpiresAt, ""},
		{"deadline equal to now", Timestamp{Present: true, Valid: true, Value: now}, time.Time{}, DeadlineExceeded},
		{"deadline in the past", Timestamp{Present: true, Valid: true, Value: now.Add(-time.Second)}, time.Time{}, DeadlineExceeded},
		{"unreadable deadline", Timestamp{Present: true, Valid: false}, time.Time{}, InvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := envelopeFor(scope)
			request.Deadline = test.deadline
			got, err := CallDeadline(request, scope, now)
			if test.wantKind != "" {
				requireRefusal(t, err, test.wantKind)
				return
			}
			if err != nil || !got.Equal(test.want) {
				t.Fatalf("deadline = %v, want %v (err %v)", got, test.want, err)
			}
		})
	}
	t.Run("expired capability", func(t *testing.T) {
		t.Parallel()
		_, err := CallDeadline(envelopeFor(scope), scope, scope.ExpiresAt)
		requireRefusal(t, err, DeadlineExceeded)
	})
}
