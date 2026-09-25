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

// Package configserver defines the seam between the config-server-backed
// cache.Client and the outside world it reads real Intents from.
//
// LocalConfigReader is exactly what that backend needs: get one Document by
// name, list every Document for a target. Nothing more. This decouples the
// backend (built behind this interface) from the real cross-repo transport
// (a unary gRPC service on config-server's colocated controller, defined in
// sdc-protos and wired up in later tickets) — the backend can be built and
// fully unit-tested against the fake in this package right now.
package configserver

import (
	"context"
	"errors"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ErrNotFound is returned by LocalConfigReader.Get when no Document exists
// for the given target/name pair.
var ErrNotFound = errors.New("configserver: document not found")

// Target scopes a Get/List call to a single southbound target, mirroring the
// TargetNamespaceKey/TargetNameKey labels config-server's controller already
// indexes Config resources by (see the ADR's "Scope note").
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

// Document is the seam's representation of one config-server Config (joined
// with its SensitiveConfig, if any). It carries exactly the fields the ADR's
// field-mapping table needs to build an importer.ImportConfigAdapter:
// name, namespace, priority, non-revertive flag, orphan flag, sensitive
// paths, and the raw config payload.
type Document struct {
	// Name is the Config resource's metadata.name — the lookup key
	// ConfigReadService uses against TargetSnapshot.Spec.Configs.
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
// Config: "<namespace>.<name>", matching config.GetGVKNSN. When Namespace is
// empty the bare Name is returned, so incomplete fixtures stay usable.
func (d *Document) IntentName() string {
	if d.Namespace == "" {
		return d.Name
	}
	return d.Namespace + "." + d.Name
}

// LocalConfigReader is the local-read seam a config-server-backed
// cache.Client depends on: get one Document by name, list every Document
// scoped to a target.
type LocalConfigReader interface {
	// Get returns the Document named name for target. It returns ErrNotFound
	// (wrapped or not) if no such Document exists.
	Get(ctx context.Context, target Target, name string) (*Document, error)
	// List returns every Document scoped to target. An empty/nil slice with a
	// nil error means the target has no Documents.
	List(ctx context.Context, target Target) ([]*Document, error)
}
