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

package schema

import (
	"context"

	schemaStore "github.com/iptecharch/schema-server/pkg/store"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"google.golang.org/grpc"
)

type localClient struct {
	schemaStore.Store
}

func NewLocalClient(store schemaStore.Store) Client {
	return &localClient{
		Store: store,
	}
}

// returns schema name, vendor, version, and files path(s)
func (c *localClient) GetSchemaDetails(ctx context.Context, in *sdcpb.GetSchemaDetailsRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaDetailsResponse, error) {
	return c.Store.GetSchemaDetails(ctx, in)
}

// lists known schemas with name, vendor, version and status
func (c *localClient) ListSchema(ctx context.Context, in *sdcpb.ListSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ListSchemaResponse, error) {
	return c.Store.ListSchema(ctx, in)
}

// returns the schema of an item identified by a gNMI-like path
func (c *localClient) GetSchema(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
	return c.Store.GetSchema(ctx, in)
}

// creates a schema
func (c *localClient) CreateSchema(ctx context.Context, in *sdcpb.CreateSchemaRequest, opts ...grpc.CallOption) (*sdcpb.CreateSchemaResponse, error) {
	return c.Store.CreateSchema(ctx, in)
}

// trigger schema reload
func (c *localClient) ReloadSchema(ctx context.Context, in *sdcpb.ReloadSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ReloadSchemaResponse, error) {
	return c.Store.ReloadSchema(ctx, in)
}

// delete a schema
func (c *localClient) DeleteSchema(ctx context.Context, in *sdcpb.DeleteSchemaRequest, opts ...grpc.CallOption) (*sdcpb.DeleteSchemaResponse, error) {
	return c.Store.DeleteSchema(ctx, in)
}

// client stream RPC to upload yang files to the server:
// - uses CreateSchema as a first message
// - then N intermediate UploadSchemaFile, initial, bytes, hash for each file
// - and ends with an UploadSchemaFinalize{}
func (c *localClient) UploadSchema(ctx context.Context, opts ...grpc.CallOption) (sdcpb.SchemaServer_UploadSchemaClient, error) {
	return nil, nil
}

// ToPath converts a list of items into a schema.proto.Path
func (c *localClient) ToPath(ctx context.Context, in *sdcpb.ToPathRequest, opts ...grpc.CallOption) (*sdcpb.ToPathResponse, error) {
	return c.Store.ToPath(ctx, in)
}

// ExpandPath returns a list of sub paths given a single path
func (c *localClient) ExpandPath(ctx context.Context, in *sdcpb.ExpandPathRequest, opts ...grpc.CallOption) (*sdcpb.ExpandPathResponse, error) {
	return c.Store.ExpandPath(ctx, in)
}

// GetSchemaElements returns the schema of each path element
func (c *localClient) GetSchemaElements(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (chan *sdcpb.SchemaElem, error) {
	return c.Store.GetSchemaElements(ctx, in)
}
