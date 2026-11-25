package api

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"locked", ErrDatastoreLocked, true},
		{"not connected", ErrNotConnected, true},
		{"resource exhausted", ErrResourceExhausted, true},
		{"deadline", context.DeadlineExceeded, true},
		{"wrapped locked", fmt.Errorf("ds1: %w", ErrDatastoreLocked), true},
		{"joined not connected", errors.Join(errors.New("x"), fmt.Errorf("a: %w", ErrNotConnected)), true},
		{"unknown datastore", ErrUnknownDatastore, false},
		{"invalid argument", ErrInvalidArgument, false},
		{"canceled", context.Canceled, false},
		{"untyped", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Fatalf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSentinelsAreDistinct(t *testing.T) {
	all := []error{ErrDatastoreLocked, ErrNotConnected, ErrResourceExhausted, ErrUnknownDatastore, ErrInvalidArgument}
	for i, a := range all {
		for j, b := range all {
			if (i == j) != errors.Is(a, b) {
				t.Fatalf("errors.Is(%v, %v) = %v", a, b, errors.Is(a, b))
			}
		}
	}
}

func TestErrorsIsThroughWrapping(t *testing.T) {
	err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrUnknownDatastore))
	if !errors.Is(err, ErrUnknownDatastore) {
		t.Fatal("wrapped sentinel not matched")
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatal("unrelated sentinel matched")
	}
}
