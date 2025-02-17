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

package utils

import (
	"fmt"
	"strconv"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// urnges represents a collection of rng (range)
type SRnges struct {
	rnges []*SRng
}

// urng represents a single unsigned range
type SRng struct {
	min int64
	max int64
}

func NewSrnges() *SRnges {
	r := &SRnges{}
	return r
}

func (r *SRng) IsInRange(value int64) bool {
	// return the result
	return r.min <= value && value <= r.max
}

func (r *SRng) String() string {
	// return the result
	return fmt.Sprintf("%d..%d", r.min, r.max)
}

func (r *SRnges) IsWithinAnyRange(val int64) bool {
	// if no ranges defined, return the tv
	if len(r.rnges) == 0 {
		return true
	}
	// check the ranges
	for _, rng := range r.rnges {
		if rng.IsInRange(val) {
			return true
		}
	}
	return false

}

func (r *SRnges) IsWithinAnyRangeString(value string) (*sdcpb.TypedValue, error) {
	if len(value) == 0 {
		return nil, nil
	}
	intValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}

	// create the TypedValue already
	tv := &sdcpb.TypedValue{
		Value: &sdcpb.TypedValue_IntVal{
			IntVal: intValue,
		},
	}
	if r.IsWithinAnyRange(intValue) {
		return tv, nil
	}
	return nil, fmt.Errorf("%q not within ranges", value)
}

func (r *SRnges) AddRange(min, max int64) {
	// to make sure the value is in the general limits of the datatype uint8|16|32|64
	// we add the min max as a seperate additional range
	r.rnges = append(r.rnges, &SRng{
		min: min,
		max: max,
	})
}

func (r *SRnges) String() string {
	sb := &strings.Builder{}
	sb.WriteString("Ranges: ")
	for _, ur := range r.rnges {
		sb.WriteString(ur.String())
		sb.WriteString(" ")
	}
	return sb.String()
}
