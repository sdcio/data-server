package schema

import (
	"context"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
)

type remoteClient struct {
	c schemapb.SchemaServerClient
}

func NewRemoteClient(cc *grpc.ClientConn) Client {
	return &remoteClient{c: schemapb.NewSchemaServerClient(cc)}
}

// returns schema name, vendor, version, and files path(s)
func (c *remoteClient) GetSchemaDetails(ctx context.Context, in *schemapb.GetSchemaDetailsRequest, opts ...grpc.CallOption) (*schemapb.GetSchemaDetailsResponse, error) {
	return c.c.GetSchemaDetails(ctx, in, opts...)
}

// lists known schemas with name, vendor, version and status
func (c *remoteClient) ListSchema(ctx context.Context, in *schemapb.ListSchemaRequest, opts ...grpc.CallOption) (*schemapb.ListSchemaResponse, error) {
	return c.c.ListSchema(ctx, in, opts...)
}

// returns the schema of an item identified by a gNMI-like path
func (c *remoteClient) GetSchema(ctx context.Context, in *schemapb.GetSchemaRequest, opts ...grpc.CallOption) (*schemapb.GetSchemaResponse, error) {
	return c.c.GetSchema(ctx, in, opts...)
}

// creates a schema
func (c *remoteClient) CreateSchema(ctx context.Context, in *schemapb.CreateSchemaRequest, opts ...grpc.CallOption) (*schemapb.CreateSchemaResponse, error) {
	return c.c.CreateSchema(ctx, in, opts...)
}

// trigger schema reload
func (c *remoteClient) ReloadSchema(ctx context.Context, in *schemapb.ReloadSchemaRequest, opts ...grpc.CallOption) (*schemapb.ReloadSchemaResponse, error) {
	return c.c.ReloadSchema(ctx, in, opts...)
}

// delete a schema
func (c *remoteClient) DeleteSchema(ctx context.Context, in *schemapb.DeleteSchemaRequest, opts ...grpc.CallOption) (*schemapb.DeleteSchemaResponse, error) {
	return c.c.DeleteSchema(ctx, in, opts...)
}

// client stream RPC to upload yang files to the server:
// - uses CreateSchema as a first message
// - then N intermediate UploadSchemaFile, initial, bytes, hash for each file
// - and ends with an UploadSchemaFinalize{}
func (c *remoteClient) UploadSchema(ctx context.Context, opts ...grpc.CallOption) (schemapb.SchemaServer_UploadSchemaClient, error) {
	return nil, nil
}

// ToPath converts a list of items into a schema.proto.Path
func (c *remoteClient) ToPath(ctx context.Context, in *schemapb.ToPathRequest, opts ...grpc.CallOption) (*schemapb.ToPathResponse, error) {
	return c.c.ToPath(ctx, in, opts...)
}

// ExpandPath returns a list of sub paths given a single path
func (c *remoteClient) ExpandPath(ctx context.Context, in *schemapb.ExpandPathRequest, opts ...grpc.CallOption) (*schemapb.ExpandPathResponse, error) {
	return c.c.ExpandPath(ctx, in, opts...)
}

// GetSchemaElements returns the schema of each path element
func (c *remoteClient) GetSchemaElements(ctx context.Context, in *schemapb.GetSchemaRequest, opts ...grpc.CallOption) (chan *schemapb.SchemaElem, error) {
	stream, err := c.c.GetSchemaElements(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	ch := make(chan *schemapb.SchemaElem)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					return
				}
				log.Errorf("stream rcv ended: %v", err)
				return
			}
			ch <- msg.GetSchema()
		}
	}()
	return ch, nil
}
