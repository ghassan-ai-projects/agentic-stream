package app

import (
	"context"
	"log/slog"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func report(ctx context.Context, reaction domain.Reaction) {
	if reaction.Verdict != nil {
		reportVerdict(ctx, *reaction.Verdict)
	}
	if len(reaction.Quarantined) > 0 {
		slog.WarnContext(ctx, "plant wire kinds quarantined: no kind file commissions them with the digest the node announces",
			"kinds", reaction.Quarantined)
	}
	if reaction.Rebirth {
		slog.WarnContext(ctx, "plant wire sequence gap: a rebirth was asked")
	}
	if reaction.Dropped != "" {
		slog.DebugContext(ctx, "plant wire record dropped", "reason", reaction.Dropped)
	}
}

func reportVerdict(ctx context.Context, verdict domain.Verdict) {
	if verdict.Accepted() {
		slog.InfoContext(ctx, "plant wire session accepted", "protocol_version", verdict.ProtocolVersion)
		return
	}
	slog.WarnContext(ctx, "plant wire session refused", "refusal", verdict.Refusal, "reason", verdict.Reason)
}
