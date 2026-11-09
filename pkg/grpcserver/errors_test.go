package grpcserver

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/datastore"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateLockedOrNotConnected(t *testing.T) {
	cases := map[string]struct {
		err      error
		wantNil  bool
		wantSame bool
		wantCode codes.Code
	}{
		"nil": {err: nil, wantNil: true},
		"datastore locked -> Aborted": {err: datastore.ErrDatastoreLocked, wantCode: codes.Aborted},
		"not connected -> Unavailable": {err: targettypes.ErrNotConnected, wantCode: codes.Unavailable},
		"wrapped not connected -> Unavailable": {
			err:      fmt.Errorf("some.datastore: %w", targettypes.ErrNotConnected),
			wantCode: codes.Unavailable,
		},
		"other error -> passthrough": {err: errors.New("boom"), wantSame: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := translateLockedOrNotConnected(tc.err)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if tc.wantSame {
				if got != tc.err {
					t.Fatalf("expected passthrough, got %v", got)
				}
				return
			}
			st, ok := status.FromError(got)
			if !ok {
				t.Fatalf("expected gRPC status, got %v", got)
			}
			if st.Code() != tc.wantCode {
				t.Fatalf("code %v, want %v", st.Code(), tc.wantCode)
			}
		})
	}
}

func TestStatusNotFound(t *testing.T) {
	err := statusNotFound(fmt.Errorf("%w ds1", api.ErrUnknownDatastore))
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}
