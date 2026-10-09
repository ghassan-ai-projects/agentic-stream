package store

import (
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"

	"modernc.org/sqlite"
)

var registerStoredTimeFunction = sync.OnceValue(func() error {
	return sqlite.RegisterDeterministicScalarFunction("stored_time_ok", 1, storedTimeOK)
})

func init() {
	_ = registerStoredTimeFunction()
}

func storedTimeOK(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	text, isText := args[0].(string)
	if !isText {
		return int64(0), nil
	}
	if _, err := kernel.ParseStoredTime(text); err != nil {
		return int64(0), nil
	}
	return int64(1), nil
}

func requireStoredTimeFunction() error {
	if err := registerStoredTimeFunction(); err != nil {
		return fmt.Errorf("register stored_time_ok: %w", err)
	}
	return nil
}
