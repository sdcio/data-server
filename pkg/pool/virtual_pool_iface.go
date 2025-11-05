package pool

// VirtualPoolI defines the public behaviour used by callers of VirtualPool.
// It intentionally exposes only the stable, public methods used by consumers
// (submission, lifecycle control and error inspection).
type VirtualPoolI interface {
    // Submit enqueues a Task into this virtual pool.
    Submit(Task) error
    // SubmitFunc convenience to submit a TaskFunc.
    SubmitFunc(TaskFunc) error
    // CloseForSubmit marks this virtual pool as no longer accepting top-level submissions.
    CloseForSubmit()
    // Wait blocks until the virtual has been closed for submit and all inflight tasks have completed.
    Wait()
    // FirstError returns the first encountered error for fail-fast virtual pools, or nil.
    FirstError() error
    // Errors returns a snapshot of collected errors for tolerant virtual pools.
    Errors() []error
    // ErrorChan returns the live channel of errors for tolerant mode, or nil for fail-fast mode.
    ErrorChan() <-chan error
}

// Ensure VirtualPool implements the interface.
var _ VirtualPoolI = (*VirtualPool)(nil)
