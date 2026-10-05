package app_test

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"time"
)

func formatTime(now time.Time) string { return domain.FormatTime(now) }
