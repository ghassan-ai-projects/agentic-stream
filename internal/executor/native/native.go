package native

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ErrInterrupt is a provider or tool interruption. Non-interactive episodes
// fail immediately when this error is returned; they never wait for input.
var ErrInterrupt = domain.ErrInterrupt

type (
	// RetryableError marks a provider failure that may consume the separate
	// provider-retry allowance.
	RetryableError = domain.RetryableError
	// Usage is provider-reported usage, treated as cumulative consumption.
	Usage = domain.Usage
	// ToolCall is a provider-proposed read operation.
	ToolCall = domain.ToolCall
	// ModelResponse is one provider turn.
	ModelResponse = domain.ModelResponse
	// ModelRequest is the immutable episode projection plus bounded observations.
	ModelRequest = domain.ModelRequest
	// ToolDefinition is the provider-facing declaration of one read capability.
	ToolDefinition = domain.ToolDefinition
	// ModelProvider is the narrow provider port used by the native executor.
	ModelProvider = domain.ModelProvider
	// Tool is a read-only episode capability.
	Tool = domain.Tool
	// ToolResult is a bounded structured observation.
	ToolResult = domain.ToolResult
	// Observation is the durable-safe projection sent to the provider.
	Observation = domain.Observation
	// ArtifactRef identifies a bounded externalized tool result.
	ArtifactRef = domain.ArtifactRef
	// ArtifactStore receives oversized tool results.
	ArtifactStore = domain.ArtifactStore
	// DeterministicProvider is the built-in provider for replay and tests.
	DeterministicProvider = domain.DeterministicProvider
	// MemoryArtifactStore is a bounded test and reference artifact store.
	MemoryArtifactStore = domain.MemoryArtifactStore
	// OpenAICompatibleProvider is the HTTP provider for chat-completions endpoints.
	OpenAICompatibleProvider = transport.OpenAICompatibleProvider
	// SQLiteEvidenceTool exposes bounded, read-only event evidence.
	SQLiteEvidenceTool = store.SQLiteEvidenceTool
	// Config controls the provider and read-only capabilities of the loop.
	Config = app.Config
	// Executor is a bounded native Go episode executor.
	Executor = app.Executor
	// BatchResult is one benchmark cell's comparator outcome.
	BatchResult = app.BatchResult
)

// NewMemoryArtifactStore creates an in-memory artifact store.
func NewMemoryArtifactStore() *MemoryArtifactStore { return domain.NewMemoryArtifactStore() }

// NewSQLiteEvidenceTool creates a scoped native evidence tool.
func NewSQLiteEvidenceTool(db *storage.DB, name, tenantID, entityID string) *SQLiteEvidenceTool {
	return store.NewSQLiteEvidenceTool(db, name, tenantID, entityID)
}

// New creates a native executor and rejects duplicate or empty tool names.
func New(cfg Config) (*Executor, error) { return app.New(cfg) } //nolint:wrapcheck // The application names each failed step.

// RunBatch drives the executor over benchmark cells, reporting every cell.
func RunBatch(ctx context.Context, executor episodes.Executor, requests []*episodes.Request, cellIDs []string) ([]BatchResult, error) {
	return app.RunBatch(ctx, executor, requests, cellIDs) //nolint:wrapcheck // The application names each failed step.
}

// RunBatchJSON is the wire form of RunBatch.
func RunBatchJSON(ctx context.Context, executor episodes.Executor, requests []*episodes.Request, cellIDs []string) ([]byte, error) {
	return app.RunBatchJSON(ctx, executor, requests, cellIDs) //nolint:wrapcheck // The application names each failed step.
}
