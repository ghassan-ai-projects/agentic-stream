package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// AuthorizeScope checks exact identity before time range and argument parsing.
func AuthorizeScope(req Envelope, scope Scope, epoch string) error {
	if err := CheckNumericRange(req); err != nil {
		return err
	}
	fence := int64(req.Fence)              //nolint:gosec // CheckNumericRange above proves the signed range.
	version := int64(req.SituationVersion) //nolint:gosec // CheckNumericRange above proves the signed range.
	if !scope.grants(req, fence, version) || (epoch != "" && scope.RuntimeEpoch != epoch) {
		return Refuse(PermissionDenied, "capability scope mismatch")
	}
	if !scope.coversTimeRange(req) {
		return Refuse(PermissionDenied, "evidence time range exceeds capability")
	}
	return nil
}

// BindArguments requires the argument entity to agree with the authenticated dimensions.
func BindArguments(req Envelope, scope Scope, trace contractsv1.TraceContext, arguments EvidenceGetArguments) (Call, error) {
	if arguments.EntityID != scope.EntityID || arguments.EntityID != req.EntityID {
		return Call{}, Refuse(PermissionDenied, "entity scope mismatch")
	}
	return Call{EpisodeID: req.EpisodeID, CallID: req.CallID, ToolName: req.ToolName, TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: scope.SituationVersion, EntityID: arguments.EntityID, Arguments: arguments, AttemptID: req.AttemptID, Fence: scope.Fence, Trace: trace, MaxRows: minNonZero(req.MaxRows, scope.MaxRows), MaxBytes: minNonZero(req.MaxBytes, scope.MaxBytes), From: req.From.Value, Until: req.Until.Value}, nil
}

// CheckNumericRange protects signed runtime identities.
func CheckNumericRange(req Envelope) error {
	if req.Fence > uint64(^uint64(0)>>1) {
		return Refuse(InvalidArgument, "fence exceeds runtime range")
	}
	if req.SituationVersion > uint64(^uint64(0)>>1) {
		return Refuse(InvalidArgument, "situation version exceeds runtime range")
	}
	return nil
}
func (scope Scope) grants(req Envelope, fence, situationVersion int64) bool {
	return scope.EpisodeID == req.EpisodeID && scope.AttemptID == req.AttemptID && scope.Fence == fence &&
		scope.TenantID == req.TenantID && scope.SituationID == req.SituationID &&
		scope.SituationVersion == situationVersion && scope.EntityID == req.EntityID &&
		contains(scope.Tools, req.ToolName) &&
		scope.Traceparent == req.Traceparent && scope.Tracestate == req.Tracestate
}
func (scope Scope) coversTimeRange(req Envelope) bool {
	from, until := req.From, req.Until
	if !from.Present || !until.Present || !from.Valid || !until.Valid {
		return false
	}
	return !until.Value.Before(from.Value) && !from.Value.Before(scope.From) && !until.Value.After(scope.Until)
}
func minNonZero(left, right uint64) uint64 {
	if left == 0 {
		return right
	}
	if left < right {
		return left
	}
	return right
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
