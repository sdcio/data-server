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

package types_test

import (
	"testing"

	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
)

func TestNewGnmiPlan_NilInput_NormalizedToEmptyPlan(t *testing.T) {
	plan := targettypes.NewGnmiPlan(nil)

	gp, ok := plan.GnmiPlan()
	if !ok {
		t.Fatal("NewGnmiPlan(nil): expected GnmiPlan() to report ok=true, got false")
	}
	if gp == nil {
		t.Fatal("NewGnmiPlan(nil): expected non-nil GnmiSetPlan")
	}
	if len(gp.Updates) != 0 || len(gp.Deletes) != 0 {
		t.Fatalf("NewGnmiPlan(nil): expected empty plan, got %d updates and %d deletes", len(gp.Updates), len(gp.Deletes))
	}
	if _, ok := plan.NetconfPlan(); ok {
		t.Fatal("NewGnmiPlan(nil): expected NetconfPlan() to report ok=false")
	}
}

func TestNewGnmiPlan_NonNilInput_PassedThrough(t *testing.T) {
	want := &targettypes.GnmiSetPlan{}
	plan := targettypes.NewGnmiPlan(want)

	gp, ok := plan.GnmiPlan()
	if !ok {
		t.Fatal("NewGnmiPlan: expected GnmiPlan() to report ok=true")
	}
	if gp != want {
		t.Fatal("NewGnmiPlan: expected the same pointer to be passed through unmodified")
	}
}
