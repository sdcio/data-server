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

package netconf

import "github.com/iptecharch/data-server/pkg/datastore/target/netconf/types"

type Driver interface {
	// Get config or state
	Get(filter string) (*types.NetconfResponse, error)
	// GetConfig
	GetConfig(source string, filter string) (*types.NetconfResponse, error)
	// EditConfig applies to the provided configuration target (candidate|running) the xml config provided in the config parameter
	EditConfig(target string, config string) (*types.NetconfResponse, error)
	// lock a target datastore
	Lock(target string) (*types.NetconfResponse, error)
	// unlock a target datastore
	Unlock(target string) (*types.NetconfResponse, error)
	// validate a source datastore
	Validate(source string) (*types.NetconfResponse, error)
	// Commit applies the candidate changes to the running config
	Commit() error
	// discard a candidate config
	Discard() error
	// Close the connection to the device
	Close() error
	// IsAlive returns true if the underlying transport driver is still open
	IsAlive() bool
}
