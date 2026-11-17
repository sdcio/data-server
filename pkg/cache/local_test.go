package cache

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/sdc-protos/tree_persist"
)

// An abandoned InstanceIntentGetAll must exit cleanly and release the store for both buffered and unbuffered errChan.
func TestLocalCache_InstanceIntentGetAll_AbandonedReleasesStore(t *testing.T) {
	for _, errCap := range []int{0, 1} {
		t.Run(fmt.Sprintf("errChanCap=%d", errCap), func(t *testing.T) {
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
			errChan := make(chan error, errCap)
			streamDone := make(chan struct{})
			go func() {
				defer close(streamDone)
				c.InstanceIntentGetAll(getCtx, name, nil, intentChan, errChan)
			}()

			<-intentChan
			time.Sleep(50 * time.Millisecond)
			cancel()

			select {
			case <-streamDone:
			case <-time.After(5 * time.Second):
				t.Fatal("InstanceIntentGetAll blocked after ctx cancellation")
			}
			if errCap > 0 {
				if err := <-errChan; !errors.Is(err, context.Canceled) {
					t.Fatalf("got err %v, want %v", err, context.Canceled)
				}
			}

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
		})
	}
}

func TestLocalCache_InstanceIntentGetAll_UnmarshalErrorReleasesStore(t *testing.T) {
	ctx := context.Background()
	raw, err := NewLocalCache(&config.CacheConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	lc := raw.(*LocalCache)

	const name = "ds"
	if err := lc.InstanceCreate(ctx, name); err != nil {
		t.Fatal(err)
	}
	if err := lc.Cache.InstanceIntentModify(ctx, name, "corrupt", []byte{0xff, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		if err := lc.InstanceIntentModify(ctx, name, &tree_persist.Intent{IntentName: fmt.Sprintf("intent-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}

	// Unbuffered errChan with no reader yet: the store stream must still be released immediately.
	getCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	intentChan := make(chan *tree_persist.Intent)
	errChan := make(chan error)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		lc.InstanceIntentGetAll(getCtx, name, nil, intentChan, errChan)
	}()

	// Drain any valid intents emitted before the corrupt one, then stop reading errChan briefly.
	readLoop:
	for {
		select {
		case _, ok := <-intentChan:
			if !ok {
				break readLoop
			}
		case <-time.After(50 * time.Millisecond):
			break readLoop
		}
	}

	// Even before errChan is read or getCtx is cancelled, InstanceDelete must not deadlock on the store.
	deleted := make(chan error, 1)
	go func() { deleted <- lc.InstanceDelete(ctx, name) }()
	select {
	case err := <-deleted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("InstanceDelete blocked on unmarshal error with unbuffered errChan")
	}

	// Reading errChan receives the unmarshal error and lets InstanceIntentGetAll finish.
	select {
	case err := <-errChan:
		if err == nil {
			t.Fatal("expected unmarshal error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for unmarshal error")
	}
	select {
	case <-streamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("InstanceIntentGetAll did not return after delivering unmarshal error")
	}
}
