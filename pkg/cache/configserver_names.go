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

// splitDatastoreName decodes a Namespaced name (e.g. "prod.srl1") into a
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

// lookupConfigName maps an owner/intent Namespaced name onto the bare Config
// resource name ConfigSnapshotService keys TargetSnapshot.Spec.Configs by.
// GetGVKNSN names ("<namespace>.<name>") are stripped when the namespace
// matches the target; a bare name is passed through unchanged so existing
// Get callers keep working. An empty rest after a matching prefix
// ("prod.") is also passed through unchanged.
func lookupConfigName(target configserver.Target, intentName string) string {
	prefix := target.Namespace + "."
	if rest, ok := strings.CutPrefix(intentName, prefix); ok && rest != "" {
		return rest
	}
	return intentName
}
