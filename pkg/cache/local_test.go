package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/sdc-protos/tree_persist"
)

// An abandoned InstanceIntentGetAll must not leave the store locked, otherwise InstanceDelete deadlocks.
func TestLocalCache_InstanceIntentGetAll_AbandonedReleasesStore(t *testing.T) {
	ctx := context.Background()
	c, err := NewLocalCache(&config.CacheConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	const name = "ds"
	if err := c.InstanceCreate(ctx, name); err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		if err := c.InstanceIntentModify(ctx, name, &tree_persist.Intent{IntentName: fmt.Sprintf("intent-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}

	getCtx, cancel := context.WithCancel(ctx)
	intentChan := make(chan *tree_persist.Intent)
	errChan := make(chan error, 1)
	go c.InstanceIntentGetAll(getCtx, name, nil, intentChan, errChan)

	<-intentChan
	// let the store producer fill its buffer and block before cancelling
	time.Sleep(100 * time.Millisecond)
	cancel()

	done := make(chan error, 1)
	go func() { done <- c.InstanceDelete(ctx, name) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("InstanceDelete blocked: store lock still held by abandoned InstanceIntentGetAll")
	}
}
