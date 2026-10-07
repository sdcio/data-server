package datastore

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// TestDriftRevertStateFullTreeForgetsScopes checks that starting a full tree revert drops the scopes of an
// older, unfinished revert, so a retry after a failure checks the whole tree as well.
func TestDriftRevertStateFullTreeForgetsScopes(t *testing.T) {
	var s driftRevertState
	p, err := sdcpb.ParsePath("/interface[name=ethernet-1/1]")
	if err != nil {
		t.Fatal(err)
	}
	s.Begin(sdcpb.NewPathSet().AddPath(&sdcpb.Path{Elem: p.GetElem()}))
	s.Fail()
	if ops.PathSetIsEmpty(s.Pending()) {
		t.Fatal("expected pending scopes after a failed partial revert")
	}

	s.Begin(nil)
	s.Fail()
	if !s.Outstanding() {
		t.Fatal("expected the revert to stay outstanding")
	}
	if !ops.PathSetIsEmpty(s.Pending()) {
		t.Fatal("a failed full tree revert must not leave scopes of an older revert to retry")
	}
}
