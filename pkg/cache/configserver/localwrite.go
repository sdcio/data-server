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

package configserver

import "context"

// LocalConfigWriter is the local-write seam a config-server-backed
// cache.Client depends on: upsert one Document by name, or remove it by
// name, scoped to a target — the write-path counterpart to
// LocalConfigReader. It writes what data-server's TransactionSet apply loop
// actually pushed south, at the same moment Cache.Type: local persists it,
// so last-applied never lags behind southbound apply (see
// pkg/cache/docs/adr/0003-config-server-write-path-real-last-applied-writes.md).
type LocalConfigWriter interface {
	// Modify upserts doc into TargetSnapshot.Spec.Configs[doc.Name] for
	// target.
	Modify(ctx context.Context, target Target, doc *Document) error
	// Delete removes name from TargetSnapshot.Spec.Configs for target.
	// Membership is a single map with no tombstone: deleting a name that
	// isn't (or is no longer) present is a no-op success, not an error, so
	// retries and out-of-order delivery stay idempotent.
	Delete(ctx context.Context, target Target, name string) error
}

// LocalConfigClient is the full local seam a config-server-backed
// cache.Client depends on: LocalConfigReader for real-Intent reads,
// LocalConfigWriter for real-Intent writes. Kept as one interface — rather
// than threading two separate seam parameters everywhere a caller needs
// both — because every concrete implementation (GRPCConfigClient,
// FakeLocalConfigClient) always satisfies both: there is exactly one
// backing resource (TargetSnapshot) and one wire client
// (config_read.ConfigSnapshotServiceClient) behind either seam.
type LocalConfigClient interface {
	LocalConfigReader
	LocalConfigWriter
}
