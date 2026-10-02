package actions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"time"
)

func (d *Dispatcher) markLeaseFailure(ctx context.Context, tx *sql.Tx, outboxID int64, commandID, code string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "UPDATE commands SET status = 'failed', updated_at = ? WHERE command_id = ?", formatTime(now), commandID); err != nil {
		return fmt.Errorf("mark invalid command failed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE outbox SET status = 'failed', last_error_code = ?, lease_owner = NULL, lease_until = NULL WHERE outbox_id = ?`, code, outboxID); err != nil {
		return fmt.Errorf("mark invalid outbox failed: %w", err)
	}
	return nil
}

func (d *Dispatcher) finishOutboxOnly(ctx context.Context, tx *sql.Tx, outboxID int64, commandStatus string, now time.Time) error {
	status := "delivered"
	if commandStatus == "outcome_unknown" {
		status = "failed"
	}
	_, err := tx.ExecContext(ctx, `UPDATE outbox SET status = ?, lease_owner = NULL, lease_until = NULL, delivered_at = CASE WHEN ? = 'delivered' THEN ? ELSE delivered_at END WHERE outbox_id = ?`, status, status, formatTime(now), outboxID)
	if err != nil {
		return fmt.Errorf("finish outbox: %w", err)
	}
	return nil
}

func verifyDigest(domain canonicaljson.Domain, document map[string]any, digest []byte) bool {
	if len(digest) != sha256.Size {
		return false
	}
	return canonicaljson.Verify(domain, document, "sha256:"+hex.EncodeToString(digest))
}

func verifyIntentDigest(document map[string]any, digest []byte) bool {
	if len(digest) != sha256.Size || !contractsv1.VerifyIntentDigest(document) {
		return false
	}
	expected, err := contractsv1.IntentDigest(document)
	if err != nil {
		return false
	}
	return expected == "sha256:"+hex.EncodeToString(digest)
}

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func documentInt(document map[string]any, key string) int {
	value, _ := document[key].(float64)
	return int(value)
}

func documentInt64(document map[string]any, key string) int64 {
	value, _ := document[key].(float64)
	return int64(value)
}

func mustDigest(document map[string]any, key string) []byte {
	value, _ := document[key].(string)
	digest, err := canonicaljson.DecodeDigest(value)
	if err != nil {
		return nil
	}
	return digest
}

func optionalJSON(value map[string]any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := canonicaljson.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode optional JSON: %w", err)
	}
	return encoded, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
