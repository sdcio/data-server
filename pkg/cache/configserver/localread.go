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
// cache.Client and the outside world it reads and writes real Intents
// through.
//
// LocalConfigReader is exactly what that backend needs for reads: get one
// Document by name, list every Document for a target. Nothing more. This
// decouples the backend from the cross-repo transport (a unary gRPC
// ConfigSnapshotService on config-server's colocated controller, defined in
// sdc-protos and implemented here by GRPCConfigClient) — the backend can
// still be fully unit-tested against the fake in this package.
//
// The Document-shaped DTO and Intent↔DTO↔ImportConfigAdapter codec live in
// pkg/cache/configsnapshot; this package only owns the remote port.
package configserver

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/cache/configsnapshot"
)

// ErrNotFound is returned by LocalConfigReader.Get when no Document exists
// for the given target/name pair.
var ErrNotFound = errors.New("configserver: document not found")

// Target, ConfigBlob, and Document are the ConfigSnapshotService interchange
// DTOs owned by configsnapshot. Aliased here so the LocalConfigClient
// contract keeps a stable import path for port callers.
type (
	Target     = configsnapshot.Target
	ConfigBlob = configsnapshot.ConfigBlob
	Document   = configsnapshot.Document
)

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
