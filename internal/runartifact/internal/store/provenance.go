package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MigrationVersion is the highest applied schema migration.
func (s *Snapshot) MigrationVersion(ctx context.Context) (int, error) {
	return s.count(ctx, "read migration version", "SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
}

// LatestSpecDigest is the digest of the tenant's newest deployment, or empty.
func (s *Snapshot) LatestSpecDigest(ctx context.Context, tenantID string) ([]byte, error) {
	return s.latestDeployment(ctx, "read latest spec digest", "spec_sha256", tenantID)
}

// LatestSpecSource is the source document of the tenant's newest deployment, or
// nil when there is none.
func (s *Snapshot) LatestSpecSource(ctx context.Context, tenantID string) ([]byte, error) {
	return s.latestDeployment(ctx, "read canonical spec", "source_json", tenantID)
}

func (s *Snapshot) latestDeployment(ctx context.Context, failure, column, tenantID string) ([]byte, error) {
	var value []byte
	err := s.tx.QueryRowContext(ctx, "SELECT "+column+" FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", tenantID).Scan(&value)
	return value, noRowsIsEmpty(err, failure)
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
	return s.count(ctx, "count device states", "SELECT COUNT(*) FROM device_reconciliation")
}

func (s *Snapshot) count(ctx context.Context, failure, query string) (int, error) {
	var count int
	if err := s.tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("%s: %w", failure, err)
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
