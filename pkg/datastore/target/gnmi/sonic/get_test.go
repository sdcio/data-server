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

package sonic_test

import (
	"testing"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/sonic"
)

func TestShapeGetRequest_ForcesDataTypeALL(t *testing.T) {
	for name, in := range map[string]gnmi.GetRequest_DataType{
		"from CONFIG": gnmi.GetRequest_CONFIG,
		"from STATE":  gnmi.GetRequest_STATE,
		"from ALL":    gnmi.GetRequest_ALL,
	} {
		t.Run(name, func(t *testing.T) {
			req := &gnmi.GetRequest{Type: in}
			sonic.ShapeGetRequest(req)
			if req.Type != gnmi.GetRequest_ALL {
				t.Errorf("ShapeGetRequest: want DataType_ALL, got %v", req.Type)
			}
		})
	}
}
