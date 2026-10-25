package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const readingSource = "plantwire.reading"

func (h *Host) checkReconciled(ctx context.Context, device authority.DeviceBoot, view domain.DeviceView) error {
	required, err := h.authority.RecordDeviceState(ctx, h.owner(), deviceState(device))
	if err != nil {
		return fmt.Errorf("record the device's boot: %w", err)
	}
	if !required {
		return nil
	}
	return h.clearByReadings(ctx, device, view)
}

func (h *Host) clearByReadings(ctx context.Context, device authority.DeviceBoot, view domain.DeviceView) error {
	cleared, err := h.authority.ResolveWithReadings(ctx, readingEvidence(h.owner(), device, view))
	if err != nil {
		return fmt.Errorf("%w: %s on %s: %w", authority.ErrReconciliationRequired, device.DeviceID, device.BootID, err)
	}
	if !cleared {
		return fmt.Errorf("%w: %s on %s has no good reading of each output newer than the reconciliation",
			authority.ErrReconciliationRequired, device.DeviceID, device.BootID)
	}
	slog.InfoContext(ctx, "plant wire device reconciliation cleared by its readings", "device", device.DeviceID, "boot_id", device.BootID)
	return nil
}

func readingEvidence(owner authority.Owner, device authority.DeviceBoot, view domain.DeviceView) authority.ReadingEvidence {
	readings := make([]authority.Reading, len(view.Readings))
	for i, held := range view.Readings {
		readings[i] = authority.Reading{Output: held.Metric, Value: held.Value, Good: held.Good, BootID: held.BootID, ReceivedAt: held.ReceivedAt}
	}
	return authority.ReadingEvidence{Device: device, Owner: owner, Source: readingSource, State: deviceState(device),
		Outputs: view.Outputs, Readings: readings}
}

func deviceState(device authority.DeviceBoot) map[string]any {
	return map[string]any{"device_id": device.DeviceID, "boot_id": device.BootID}
}
