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

// Package configserver defines the Go port between the config-server-backed
// cache.Client and the ConfigSnapshotService wire it reads and writes real
// Intents through.
//
// ConfigSnapshotClient is exactly what that backend needs: Get/List/Modify/
// Delete against Document-shaped DTOs. Nothing more. This decouples the
// backend from the cross-repo transport (a unary gRPC ConfigSnapshotService
// on config-server's colocated controller, defined in sdc-protos and
// implemented here by GRPCConfigClient) — the backend can still be fully
// unit-tested against the fake in this package.
//
// Capability segregation (IntentReader / IntentWriter) lives one layer up on
// cache.Client — see ADR 0002 / 0003. The Document-shaped DTO and
// Intent↔DTO↔ImportConfigAdapter codec live in pkg/cache/configsnapshot;
// this package owns only the remote port.
package configserver

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/cache/configsnapshot"
)

// ErrNotFound is returned by ConfigSnapshotClient.Get when no Document exists
// for the given target/name pair.
var ErrNotFound = errors.New("configserver: document not found")

// Target, ConfigBlob, and Document are the ConfigSnapshotService interchange
// DTOs owned by configsnapshot. Aliased here so the ConfigSnapshotClient
// contract keeps a stable import path for port callers.
type (
	Target     = configsnapshot.Target
	ConfigBlob = configsnapshot.ConfigBlob
	Document   = configsnapshot.Document
)

// ConfigSnapshotClient is the Go port over ConfigSnapshotService: get/list/
// upsert/delete Document entries scoped to a target. Distinct from the
// generated config_read.ConfigSnapshotServiceClient stub. Implemented by
// GRPCConfigClient and FakeConfigSnapshotClient.
//
// Modify/Delete write what data-server's TransactionSet apply loop actually
// pushed south, at the same moment Cache.Type: local persists it, so
// last-applied never lags behind southbound apply (see
// pkg/cache/docs/adr/0003-config-server-write-path-real-last-applied-writes.md).
type ConfigSnapshotClient interface {
	// Get returns the Document named name for target. It returns ErrNotFound
	// (wrapped or not) if no such Document exists.
	Get(ctx context.Context, target Target, name string) (*Document, error)
	// List returns every Document scoped to target. An empty/nil slice with a
	// nil error means the target has no Documents.
	List(ctx context.Context, target Target) ([]*Document, error)
	// Modify upserts doc into TargetSnapshot.Spec.Configs[doc.Name] for
	// target.
	Modify(ctx context.Context, target Target, doc *Document) error
	// Delete removes name from TargetSnapshot.Spec.Configs for target.
	// Membership is a single map with no tombstone: deleting a name that
	// isn't (or is no longer) present is a no-op success, not an error, so
	// retries and out-of-order delivery stay idempotent.
	Delete(ctx context.Context, target Target, name string) error
}
