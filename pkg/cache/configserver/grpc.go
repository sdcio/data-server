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

// GRPCConfigReader implements LocalConfigReader over the real
// config_read.ConfigReadService, the localhost-bound gRPC surface ticket 07
// serves from inside the colocated config-server controller.
//
// It takes a grpc.ClientConnInterface rather than dialing one itself,
// mirroring pkg/schema.NewRemoteClient: dial address/credentials are the
// caller's concern (Dial below is one convenience constructor for the
// localhost/insecure case this service is always deployed as), so nothing
// here is hardcoded.
type GRPCConfigReader struct {
	client config_read.ConfigReadServiceClient
}

// NewGRPCConfigReader returns a LocalConfigReader that calls the
// ConfigReadService over cc.
func NewGRPCConfigReader(cc grpc.ClientConnInterface) *GRPCConfigReader {
	return &GRPCConfigReader{client: config_read.NewConfigReadServiceClient(cc)}
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

// Get calls ConfigReadService.Get, mapping a NotFound gRPC status to
// ErrNotFound per the LocalConfigReader contract; any other error
// propagates unwrapped.
func (r *GRPCConfigReader) Get(ctx context.Context, target Target, name string) (*Document, error) {
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

// List calls ConfigReadService.List, mapping every returned ConfigEntry into
// a Document.
func (r *GRPCConfigReader) List(ctx context.Context, target Target) ([]*Document, error) {
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
		Priority:       e.GetPriority(),
		NonRevertive:   e.GetNonRevertive(),
		Orphan:         e.GetOrphan(),
		SensitivePaths: e.GetSensitivePaths(),
		Config:         config,
	}
}

var _ LocalConfigReader = (*GRPCConfigReader)(nil)
