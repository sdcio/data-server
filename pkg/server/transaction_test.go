package server

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sdcio/data-server/pkg/datastore"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateInternalToGrpcError(t *testing.T) {
	cases := map[string]struct {
		err      error
		wantNil  bool
		wantSame bool // returned unchanged (non-status passthrough)
		wantCode codes.Code
	}{
		"nil": {
			err:     nil,
			wantNil: true,
		},
		"datastore locked -> Aborted": {
			err:      datastore.ErrDatastoreLocked,
			wantCode: codes.Aborted,
		},
		"not connected -> Unavailable": {
			err:      targettypes.ErrNotConnected,
			wantCode: codes.Unavailable,
		},
		"wrapped not connected -> Unavailable": {
			err:      fmt.Errorf("some.datastore: %w", targettypes.ErrNotConnected),
			wantCode: codes.Unavailable,
		},
		"other error -> passthrough": {
			err:      errors.New("boom"),
			wantSame: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := translateInternalToGrpcError(tc.err)

			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}

			if tc.wantSame {
				if got != tc.err {
					t.Fatalf("expected passthrough of the same error, got %v", got)
				}
				return
			}

			st, ok := status.FromError(got)
			if !ok {
				t.Fatalf("expected a gRPC status error, got %v", got)
			}
			if st.Code() != tc.wantCode {
				t.Fatalf("expected code %v, got %v (%v)", tc.wantCode, st.Code(), got)
			}
		})
	}
}
