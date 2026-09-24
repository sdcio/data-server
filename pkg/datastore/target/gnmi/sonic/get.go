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

package sonic

import "github.com/openconfig/gnmi/proto/gnmi"

// ShapeGetRequest adapts a Get request for the sonic device-profile in place.
//
// SONiC's gNMI Get only implements DataType_ALL; CONFIG/STATE are rejected
// with "unsupported request type". translib has no config-vs-state split on
// Get, so ALL is the only value that ever makes sense for this profile.
func ShapeGetRequest(req *gnmi.GetRequest) {
	req.Type = gnmi.GetRequest_ALL
}
