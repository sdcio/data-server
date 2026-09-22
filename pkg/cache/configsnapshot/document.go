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

// Package configsnapshot owns the last-applied Intent ↔ ConfigSnapshotService
// DTO ↔ IntentAdapter interchange: the Document-shaped wire form plus
// flatten, blob merge, and NewImportAdapter. Document is a Go DTO name only
// — not a domain term (see pkg/cache/CONTEXT.md Last-applied).
//
// The remote Get/List/Modify/Delete port stays in pkg/cache/configserver;
// ConfigServerCache only orchestrates codec ↔ client.
package configsnapshot

import sdcpb "github.com/sdcio/sdc-protos/sdcpb"

// Target scopes a Document to a single southbound target, mirroring the
// TargetNamespaceKey/TargetNameKey labels config-server's controller indexes
// Config resources by.
type Target struct {
	Namespace string
	Name      string
}

// ConfigBlob is a single raw path + JSON value entry from a Document's
// config payload — shaped so it parses cleanly into the same form
// importer/json.JsonTreeImporter already consumes.
type ConfigBlob struct {
	Path string
	// Value is the raw JSON encoding of the value at Path.
	Value []byte
}

// Document is the ConfigSnapshotService interchange representation of one
// config-server Config (joined with its SensitiveConfig, if any). It carries
// exactly the fields needed to build an importer.IntentAdapter: name,
// namespace, priority, non-revertive flag, orphan flag, sensitive paths, and
// the raw config payload.
type Document struct {
	// Name is the Config resource's metadata.name — the lookup key
	// ConfigSnapshotService uses against TargetSnapshot.Spec.Configs.
	Name string
	// Namespace is the Config's Kubernetes namespace, populated from
	// ConfigEntry.Namespace. Together with Name it forms the owner string
	// config-server uses on TransactionSet (config.GetGVKNSN).
	Namespace      string
	Priority       int32
	NonRevertive   bool
	Orphan         bool
	SensitivePaths []*sdcpb.Path
	Config         []*ConfigBlob
}

// IntentName is the owner name data-server and config-server share for a
// Config: a Namespaced name ("<namespace>.<name>"), matching
// config.GetGVKNSN. When Namespace is empty the bare Name is returned, so
// incomplete fixtures stay usable.
func (d *Document) IntentName() string {
	return namespacedName(d.Namespace, d.Name)
}

// namespacedName joins namespace and name with a single '.' (Namespaced name).
// An empty namespace returns the bare name.
func namespacedName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "." + name
}
