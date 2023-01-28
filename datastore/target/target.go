package target

import "context"

type Target interface {
	Get()
	Set()
	Subscribe()
	//
	Sync(ctx context.Context)
}
