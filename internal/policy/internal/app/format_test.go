package app_test

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func formatTime(now time.Time) string { return domain.FormatTime(now) }
