package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MigrationVersion is the highest applied schema migration.
func (s *Snapshot) MigrationVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("read migration version: %w", err)
	}
	return version, nil
}

// LatestSpecDigest is the digest of the tenant's newest deployment, or empty.
func (s *Snapshot) LatestSpecDigest(ctx context.Context, tenantID string) ([]byte, error) {
	var digest []byte
	err := s.tx.QueryRowContext(ctx, "SELECT spec_sha256 FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", tenantID).Scan(&digest)
	return digest, noRowsIsEmpty(err, "read latest spec digest")
}

// LatestSpecSource is the source document of the tenant's newest deployment, or
// nil when there is none.
func (s *Snapshot) LatestSpecSource(ctx context.Context, tenantID string) ([]byte, error) {
	var source []byte
	err := s.tx.QueryRowContext(ctx, "SELECT source_json FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", tenantID).Scan(&source)
	return source, noRowsIsEmpty(err, "read canonical spec")
}

// PolicyEvaluation is the policy version and digest of the tenant's newest
// evaluation.
type PolicyEvaluation struct {
	Version string
	Digest  string
}

// LatestPolicyEvaluation returns the tenant's newest policy evaluation, or the
// zero value when none exists.
func (s *Snapshot) LatestPolicyEvaluation(ctx context.Context, tenantID string) (PolicyEvaluation, error) {
	var evaluation PolicyEvaluation
	err := s.tx.QueryRowContext(ctx, `SELECT pe.policy_version, pe.policy_digest
		FROM policy_evaluations pe JOIN intents i ON i.intent_id = pe.intent_id
		WHERE i.tenant_id = ? ORDER BY pe.evaluated_at DESC, pe.evaluation_id DESC LIMIT 1`, tenantID).Scan(&evaluation.Version, &evaluation.Digest)
	return evaluation, noRowsIsEmpty(err, "read current policy input")
}

// DeviceStateCount is the number of devices that have reported state.
func (s *Snapshot) DeviceStateCount(ctx context.Context) (int, error) {
	var count int
	if err := s.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM device_reconciliation").Scan(&count); err != nil {
		return 0, fmt.Errorf("count device states: %w", err)
	}
	return count, nil
}

// DeviceState is the last reported state document of the named device, or of
// the first device by id when deviceID is empty; nil when none reported.
func (s *Snapshot) DeviceState(ctx context.Context, deviceID string) ([]byte, error) {
	query, args := "SELECT state_json FROM device_reconciliation ORDER BY device_id LIMIT 1", []any(nil)
	if deviceID != "" {
		query, args = "SELECT state_json FROM device_reconciliation WHERE device_id = ?", []any{deviceID}
	}
	var state []byte
	err := s.tx.QueryRowContext(ctx, query, args...).Scan(&state)
	return state, noRowsIsEmpty(err, "read device state")
}

func noRowsIsEmpty(err error, what string) error {
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}
