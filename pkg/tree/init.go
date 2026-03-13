package tree

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree/api"
)

func init() {
	// Register the NewEntry factory with the api package
	// This allows processors to create entries without importing the tree package directly
	api.RegisterEntryFactory(func(ctx context.Context, parent api.Entry, pathElemName string, tc api.TreeContext) (api.Entry, error) {
		return NewEntry(ctx, parent, pathElemName, tc)
	})
}
