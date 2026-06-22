// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package target_test

import (
	"context"
	"testing"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target"
)

// TestNew_EmptyTypeReturnsError verifies that target.New called with an empty
// SBI type returns a non-nil error instead of silently becoming a noop target.
func TestNew_EmptyTypeReturnsError(t *testing.T) {
	cfg := &config.SBI{Type: ""}

	_, err := target.New(context.Background(), "ds1", cfg, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty SBI type, got nil")
	}
}

// TestNew_NoopTypeSucceeds verifies that target.New called with SBI type "noop"
// still returns a working target without error.
func TestNew_NoopTypeSucceeds(t *testing.T) {
	cfg := &config.SBI{Type: "noop"}

	tgt, err := target.New(context.Background(), "ds-noop", cfg, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error for noop type, got: %v", err)
	}
	if tgt == nil {
		t.Fatal("expected non-nil target for noop type")
	}
}
