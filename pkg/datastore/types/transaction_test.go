package types

import (
	"testing"

	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// pathAndUpdate builds a minimal treetypes.PathAndUpdate, sufficient for these tests, which only
// care about how many updates ended up in each TransactionIntent, not their content.
func newTestPath(name string) *sdcpb.Path {
	return &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: name}}}
}

func TestGetRollbackTransaction_ReplaysOldIntentsUnconditionally(t *testing.T) {
	tm := NewTransactionManager(nil)
	tr := NewTransaction("t1", tm)
	tr.SetTimeout(nil, 0) //nolint:staticcheck // ctx unused by the timer until Start is called

	old := NewTransactionIntent("intent-a", 10)
	old.AddUpdate(treetypes.NewPathAndUpdate(newTestPath("a"), nil))
	if err := tr.AddTransactionIntent(old, TransactionIntentOld); err != nil {
		t.Fatalf("failed to add old intent: %v", err)
	}

	rb := tr.GetRollbackTransaction()

	newIntents := rb.GetNewIntents()
	if len(newIntents) != 1 {
		t.Fatalf("expected 1 replayed intent, got %d", len(newIntents))
	}
	if _, ok := newIntents["intent-a"]; !ok {
		t.Fatalf("expected old intent 'intent-a' to be replayed as a new intent on the rollback transaction")
	}
	if !rb.IsRollback() {
		t.Fatalf("expected rollback transaction to be marked as such")
	}
}

func TestGetRollbackTransaction_NoReplace_ReplaceStaysNil(t *testing.T) {
	tm := NewTransactionManager(nil)
	tr := NewTransaction("t1", tm)
	tr.SetTimeout(nil, 0) //nolint:staticcheck

	// no replace set on the original transaction (SetReplace never called / left nil)
	if tr.GetReplace() != nil {
		t.Fatalf("expected no replace on original transaction by default")
	}

	rb := tr.GetRollbackTransaction()

	if rb.GetReplace() != nil {
		t.Fatalf("expected rollback of a plain intents-only transaction to have no .replace, got %v", rb.GetReplace())
	}
}

func TestGetRollbackTransaction_WithReplace_SetsReplaceFromOldRunning(t *testing.T) {
	tm := NewTransactionManager(nil)
	tr := NewTransaction("t1", tm)
	tr.SetTimeout(nil, 0) //nolint:staticcheck

	// simulate the original transaction having been a replace
	tr.SetReplace(NewTransactionIntent("replace", 1))

	// simulate the pre-replace Running snapshot having been captured
	tr.GetOldRunning().AddUpdate(treetypes.NewPathAndUpdate(newTestPath("running-leaf"), nil))

	rb := tr.GetRollbackTransaction()

	if rb.GetReplace() == nil {
		t.Fatalf("expected rollback of a replace transaction to have .replace set")
	}
	if rb.GetReplace() != tr.GetOldRunning() {
		t.Fatalf("expected rollback .replace to be the original transaction's oldRunning snapshot")
	}
	if len(rb.GetReplace().GetUpdates()) != 1 {
		t.Fatalf("expected rollback .replace to carry the captured pre-replace Running updates, got %d", len(rb.GetReplace().GetUpdates()))
	}
}
