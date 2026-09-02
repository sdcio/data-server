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

import (
	"context"

	"github.com/sdcio/sdc-protos/config_read"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// GRPCConfigClient implements LocalConfigClient over the real
// config_read.ConfigSnapshotService, the localhost-bound gRPC surface
// ticket 07 serves from inside the colocated config-server controller.
// Get/List/Modify/Delete are all one RPC service over one resource
// (TargetSnapshot), so one generated client structurally satisfies both the
// LocalConfigReader and LocalConfigWriter halves of the seam (see ADR
// 0003's "Wire contract" section).
//
// It takes a grpc.ClientConnInterface rather than dialing one itself,
// mirroring pkg/schema.NewRemoteClient: dial address/credentials are the
// caller's concern (Dial below is one convenience constructor for the
// localhost/insecure case this service is always deployed as), so nothing
// here is hardcoded.
type GRPCConfigClient struct {
	client config_read.ConfigSnapshotServiceClient
}

// NewGRPCConfigClient returns a LocalConfigClient that calls the
// ConfigSnapshotService over cc.
func NewGRPCConfigClient(cc grpc.ClientConnInterface) *GRPCConfigClient {
	return &GRPCConfigClient{client: config_read.NewConfigSnapshotServiceClient(cc)}
}

// Dial establishes an insecure gRPC connection to address, the shape every
// caller needs for this service: it is always localhost-bound, colocated in
// the same pod as the controller that serves it (see the ADR's Scope note).
// address (and thus which port to reach) is entirely caller-supplied, never
// hardcoded here.
func Dial(address string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	dialOpts := opts
	if len(dialOpts) == 0 {
		dialOpts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	return grpc.NewClient(address, dialOpts...)
}

// Get calls ConfigSnapshotService.Get, mapping a NotFound gRPC status to
// ErrNotFound per the LocalConfigReader contract; any other error
// propagates unwrapped.
func (r *GRPCConfigClient) Get(ctx context.Context, target Target, name string) (*Document, error) {
	rsp, err := r.client.Get(ctx, &config_read.GetConfigRequest{
		TargetNamespace: target.Namespace,
		TargetName:      target.Name,
		Name:            name,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return documentFromEntry(rsp.GetConfig()), nil
}

// List calls ConfigSnapshotService.List, mapping every returned ConfigEntry
// into a Document.
func (r *GRPCConfigClient) List(ctx context.Context, target Target) ([]*Document, error) {
	rsp, err := r.client.List(ctx, &config_read.ListConfigRequest{
		TargetNamespace: target.Namespace,
		TargetName:      target.Name,
	})
	if err != nil {
		return nil, err
	}
	entries := rsp.GetConfig()
	docs := make([]*Document, 0, len(entries))
	for _, e := range entries {
		docs = append(docs, documentFromEntry(e))
	}
	return docs, nil
}

// documentFromEntry maps a config_read.ConfigEntry onto the seam's Document
// shape, field for field — no further translation happens here, that's
// NewImportAdapter's job.
func documentFromEntry(e *config_read.ConfigEntry) *Document {
	blobs := e.GetConfig()
	config := make([]*ConfigBlob, 0, len(blobs))
	for _, b := range blobs {
		config = append(config, &ConfigBlob{Path: b.GetPath(), Value: b.GetValue()})
	}
	return &Document{
		Name:           e.GetName(),
		Namespace:      e.GetNamespace(),
		Priority:       e.GetPriority(),
		NonRevertive:   e.GetNonRevertive(),
		Orphan:         e.GetOrphan(),
		SensitivePaths: e.GetSensitivePaths(),
		Config:         config,
	}
}

// Modify calls ConfigSnapshotService.Modify, mapping doc onto the wire
// ConfigEntry shape — the inverse of documentFromEntry.
func (r *GRPCConfigClient) Modify(ctx context.Context, target Target, doc *Document) error {
	blobs := make([]*config_read.ConfigBlob, 0, len(doc.Config))
	for _, b := range doc.Config {
		blobs = append(blobs, &config_read.ConfigBlob{Path: b.Path, Value: b.Value})
	}
	_, err := r.client.Modify(ctx, &config_read.ModifyConfigRequest{
		TargetNamespace: target.Namespace,
		TargetName:      target.Name,
		Config: &config_read.ConfigEntry{
			Name:           doc.Name,
			Namespace:      doc.Namespace,
			NonRevertive:   doc.NonRevertive,
			Orphan:         doc.Orphan,
			Priority:       doc.Priority,
			SensitivePaths: doc.SensitivePaths,
			Config:         blobs,
		},
	})
	return err
}

// Delete calls ConfigSnapshotService.Delete. The server side already treats
// a missing key/snapshot as a no-op success (see the config-server ticket
// 02 handler), so no NotFound mapping is needed here.
func (r *GRPCConfigClient) Delete(ctx context.Context, target Target, name string) error {
	_, err := r.client.Delete(ctx, &config_read.DeleteConfigRequest{
		TargetNamespace: target.Namespace,
		TargetName:      target.Name,
		Name:            name,
	})
	return err
}

var _ LocalConfigClient = (*GRPCConfigClient)(nil)
