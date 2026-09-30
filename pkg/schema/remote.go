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
	"strings"

	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
)

type remoteClient struct {
	// the schema server client
	c sdcpb.SchemaServerClient
}

func NewRemoteClient(cc grpc.ClientConnInterface) Client {
	return &remoteClient{
		c: sdcpb.NewSchemaServerClient(cc),
	}
}

// returns schema name, vendor, version, and files path(s)
func (c *remoteClient) GetSchemaDetails(ctx context.Context, in *sdcpb.GetSchemaDetailsRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaDetailsResponse, error) {
	return c.c.GetSchemaDetails(ctx, in, opts...)
}

// lists known schemas with name, vendor, version and status
func (c *remoteClient) ListSchema(ctx context.Context, in *sdcpb.ListSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ListSchemaResponse, error) {
	return c.c.ListSchema(ctx, in, opts...)
}

// returns the schema of an item identified by a gNMI-like path
func (c *remoteClient) GetSchema(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
	return c.c.GetSchema(ctx, in, opts...)
}

// creates a schema
func (c *remoteClient) CreateSchema(ctx context.Context, in *sdcpb.CreateSchemaRequest, opts ...grpc.CallOption) (*sdcpb.CreateSchemaResponse, error) {
	return c.c.CreateSchema(ctx, in, opts...)
}

// trigger schema reload
func (c *remoteClient) ReloadSchema(ctx context.Context, in *sdcpb.ReloadSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ReloadSchemaResponse, error) {
	return c.c.ReloadSchema(ctx, in, opts...)
}

// delete a schema
func (c *remoteClient) DeleteSchema(ctx context.Context, in *sdcpb.DeleteSchemaRequest, opts ...grpc.CallOption) (*sdcpb.DeleteSchemaResponse, error) {
	return c.c.DeleteSchema(ctx, in, opts...)
}

// client stream RPC to upload yang files to the server:
// - uses CreateSchema as a first message
// - then N intermediate UploadSchemaFile, initial, bytes, hash for each file
// - and ends with an UploadSchemaFinalize{}
func (c *remoteClient) UploadSchema(ctx context.Context, opts ...grpc.CallOption) (sdcpb.SchemaServer_UploadSchemaClient, error) {
	return nil, nil
}

// ToPath converts a list of items into a schema.proto.Path
func (c *remoteClient) ToPath(ctx context.Context, in *sdcpb.ToPathRequest, opts ...grpc.CallOption) (*sdcpb.ToPathResponse, error) {
	return c.c.ToPath(ctx, in, opts...)
}

// ExpandPath returns a list of sub paths given a single path
func (c *remoteClient) ExpandPath(ctx context.Context, in *sdcpb.ExpandPathRequest, opts ...grpc.CallOption) (*sdcpb.ExpandPathResponse, error) {
	return c.c.ExpandPath(ctx, in, opts...)
}

// GetSchemaElements returns the schema of each path element
func (c *remoteClient) GetSchemaElements(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (chan *sdcpb.SchemaElem, error) {
	log := logf.FromContext(ctx)
	ch := make(chan *sdcpb.SchemaElem)
	stream, err := c.c.GetSchemaElements(ctx, in, opts...)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					return
				}
				log.Error(err, "stream rcv ended")
				return
			}
			ch <- msg.GetSchema()
		}
	}()
	return ch, nil
}
