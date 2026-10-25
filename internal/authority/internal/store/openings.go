package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const latestOpeningSQL = `
		SELECT details_json, occurred_at FROM device_authority_events
		WHERE target = ? AND device_id = ? AND boot_id = ? AND event_type = ?
		ORDER BY event_id DESC LIMIT 1`

func (t *Tx) LatestOpening(ctx context.Context, device domain.DeviceBoot) (domain.Opening, bool, error) {
	var details []byte
	var at string
	err := t.tx.QueryRowContext(ctx, latestOpeningSQL, device.DeviceID, device.DeviceID, device.BootID,
		string(domain.EventReconciliationOpened)).Scan(&details, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Opening{}, false, nil
	}
	if err != nil {
		return domain.Opening{}, false, fmt.Errorf("load the reconciliation opening: %w", err)
	}
	opening, err := openingOf(at, details)
	return opening, err == nil, err
}

func openingOf(at string, details []byte) (domain.Opening, error) {
	openedAt, err := kernel.ParseStoredTime(at)
	if err != nil {
		return domain.Opening{}, fmt.Errorf("read the reconciliation opening time: %w", err)
	}
	return domain.NewOpening(openedAt, details)
}
