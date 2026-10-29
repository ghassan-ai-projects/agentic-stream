package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

const maxCachedSamples = 1_000_000

type operatorScope struct {
	partitionID int
	entityID    string
}

type operatorStateCache struct {
	states map[operatorScope]*operators.PartitionState
	sizes  map[operatorScope]int
	total  int
}

func (c *operatorStateCache) recall(scope operatorScope) (*operators.PartitionState, bool) {
	state, ok := c.states[scope]
	return state, ok
}

func (c *operatorStateCache) remember(scope operatorScope, state *operators.PartitionState) {
	if state == nil {
		return
	}
	size := sampleCount(state)
	if c.total-c.sizes[scope]+size > maxCachedSamples {
		c.forget()
	}
	if c.states == nil {
		c.states, c.sizes = make(map[operatorScope]*operators.PartitionState), make(map[operatorScope]int)
	}
	c.total += size - c.sizes[scope]
	c.states[scope], c.sizes[scope] = state, size
}

func (c *operatorStateCache) forget() {
	*c = operatorStateCache{}
}

func sampleCount(state *operators.PartitionState) int {
	var count int
	for _, states := range state.OperatorStates {
		for _, blob := range states {
			if blob.Window != nil {
				count += len(blob.Window.Samples)
			}
		}
	}
	return count
}

func (s *Service) entityOperatorState(ctx context.Context, tx *store.Tx, scope operatorScope) (*operators.PartitionState, error) {
	if state, ok := s.operatorStates.recall(scope); ok {
		return state, nil
	}
	state, err := tx.LoadOperatorState(ctx, scope.partitionID, scope.entityID)
	if err != nil {
		return nil, fmt.Errorf("load operator state: %w", err)
	}
	return state, nil
}
