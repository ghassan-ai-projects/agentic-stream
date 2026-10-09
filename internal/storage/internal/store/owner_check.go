package store

import (
	"context"
	"database/sql"
)

type OwnerCheck = func(context.Context, *sql.Tx, string) error
