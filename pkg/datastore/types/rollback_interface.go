package types

import (
	"context"
)

type RollbackInterface interface {
	TransactionRollback(ctx context.Context, transaction *Transaction, dryRun bool) error
}
