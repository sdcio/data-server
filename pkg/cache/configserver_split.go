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

package cache

import (
	"errors"
	"strings"

	"github.com/sdcio/data-server/pkg/cache/configserver"
)

// ErrMalformedDatastoreName is returned by splitDatastoreName when a
// datastore name doesn't decode into a non-empty namespace and a non-empty
// name.
var ErrMalformedDatastoreName = errors.New("configserver cache: malformed datastore name")

// splitDatastoreName decodes a datastore name (e.g. "prod.srl1") into a
// configserver.Target, mirroring config-server's own encoding
// (storebackend.Key.String(), "<target namespace>.<target name>"). It splits
// on the first '.' only, since a Kubernetes namespace is always a DNS-1123
// label (no dots) while a target's bare name may itself legally contain
// dots. It returns ErrMalformedDatastoreName whenever there's no dot at all,
// or when either resulting segment is empty.
func splitDatastoreName(cacheInstanceName string) (configserver.Target, error) {
	namespace, name, found := strings.Cut(cacheInstanceName, ".")
	if !found || namespace == "" || name == "" {
		return configserver.Target{}, ErrMalformedDatastoreName
	}
	return configserver.Target{Namespace: namespace, Name: name}, nil
}
