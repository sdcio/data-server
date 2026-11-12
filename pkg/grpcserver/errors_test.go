package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/datastore"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTransactionError(t *testing.T) {
	plain := errors.New("boom")
	cases := map[string]struct {
		err  error
		code codes.Code // codes.OK means not a status error
		msg  string
	}{
		"unknown datastore":     {err: fmt.Errorf("%w ds", api.ErrUnknownDatastore), code: codes.NotFound, msg: "unknown datastore ds"},
		"locked":                {err: datastore.ErrDatastoreLocked, code: codes.Aborted},
		"wrapped locked":        {err: fmt.Errorf("x: %w", api.ErrDatastoreLocked), code: codes.Aborted},
		"not connected":         {err: targettypes.ErrNotConnected, code: codes.Unavailable},
		"wrapped not connected": {err: fmt.Errorf("ds: %w", targettypes.ErrNotConnected), code: codes.Unavailable},
		"resource exhausted":    {err: fmt.Errorf("x: %w", api.ErrResourceExhausted), code: codes.ResourceExhausted},
		"deadline exceeded":     {err: fmt.Errorf("x: %w", context.DeadlineExceeded), code: codes.DeadlineExceeded},
		"invalid stays untyped": {err: fmt.Errorf("%w: priority too high", api.ErrInvalidArgument), msg: "priority too high"},
		"other passes through":  {err: plain, msg: "boom"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := transactionError(tc.err)
			st, isStatus := status.FromError(got)
			if tc.code == codes.OK {
				if isStatus {
					t.Fatalf("want untyped error, got status %v", st)
				}
			} else if !isStatus || st.Code() != tc.code {
				t.Fatalf("got %v, want code %v", got, tc.code)
			}
			if tc.msg != "" && !isStatus && got.Error() != tc.msg {
				t.Fatalf("message %q, want %q", got.Error(), tc.msg)
			}
			if tc.msg != "" && isStatus && st.Message() != tc.msg {
				t.Fatalf("message %q, want %q", st.Message(), tc.msg)
			}
		})
	}
	if transactionError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	if got := transactionError(plain); got != plain {
		t.Fatal("untyped error must pass through unchanged")
	}
}

func TestInvalidArgumentTrimsSentinel(t *testing.T) {
	st, _ := status.FromError(invalidArgument(fmt.Errorf("%w: datastore x already exists", api.ErrInvalidArgument)))
	if st.Code() != codes.InvalidArgument || st.Message() != "datastore x already exists" {
		t.Fatalf("got %v", st)
	}
}
