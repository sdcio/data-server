package utils

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Debug stress test: tracks Put errors and queue length to find where items disappear.
func TestWorkerPoolQueue_StressDebug(t *testing.T) {
	q := NewWorkerPoolQueue[int]()
	const producers = 8
	const consumers = 32
	const perProducer = 10000
	total := int64(producers * perProducer)

	var produced int64         // attempts (kept for reference)
	var putErrors int64        // Put returned ErrClosed or other error
	var enqueuedReported int64 // q.Len() snapshot after producers done
	var consumed int64

	var wg sync.WaitGroup

	// Consumers
	wg.Add(consumers)
	for i := 0; i < consumers; i++ {
		go func(id int) {
			defer wg.Done()
			for {
				v, ok := q.Get()
				if !ok {
					return
				}
				_ = v
				atomic.AddInt64(&consumed, 1)
			}
		}(i)
	}

	// Producers
	var pwg sync.WaitGroup
	pwg.Add(producers)
	for p := 0; p < producers; p++ {
		go func(base int) {
			defer pwg.Done()
			for i := 0; i < perProducer; i++ {
				atomic.AddInt64(&produced, 1)
				if err := q.Put(base*perProducer + i); err != nil {
					// record Put errors
					atomic.AddInt64(&putErrors, 1)
				}
			}
		}(p)
	}

	// wait for producers to finish
	pwg.Wait()

	// snapshot how many the queue reports as enqueued
	enqueuedReported = int64(q.Len())

	// let consumers run for a bit to drain
	time.Sleep(200 * time.Millisecond)

	// close queue so remaining consumers exit and we can get final counts
	q.Close()
	wg.Wait()

	// final snapshot
	finalLen := int64(q.Len())
	p := atomic.LoadInt64(&produced)
	pe := atomic.LoadInt64(&putErrors)
	c := atomic.LoadInt64(&consumed)

	fmt.Printf("DEBUG: producedAttempts=%d putErrors=%d enqueuedReported(after producers)=%d finalReportedLen=%d consumed=%d totalExpected=%d\n",
		p, pe, enqueuedReported, finalLen, c, total)

	if pe != 0 {
		t.Fatalf("Put returned errors: %d (see debug print)", pe)
	}
	if c != total {
		t.Fatalf("consumed %d, want %d (see debug print)", c, total)
	}
}
