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
	"fmt"
	"strings"

	"github.com/jellydator/ttlcache/v3"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/utils"
)

type cacheKey struct {
	Name    string
	Vendor  string
	Version string
	Path    string
}

type remoteClient struct {
	// the schema server client
	c sdcpb.SchemaServerClient
	// cache used to store SchemaResponses if enabled.
	schemaCache *ttlcache.Cache[cacheKey, *sdcpb.GetSchemaResponse]
	// remote cache config.
	cacheConfig *config.RemoteSchemaCache
	// cacheGet options, only `WithDisableTouchOnHit` is used now.
	getOpts []ttlcache.Option[cacheKey, *sdcpb.GetSchemaResponse]
}

func NewRemoteClient(cc *grpc.ClientConn, cacheConfig *config.RemoteSchemaCache) Client {
	// no cache
	if cacheConfig == nil {
		return &remoteClient{
			c: sdcpb.NewSchemaServerClient(cc),
		}
	}
	// rc with cache
	rc := &remoteClient{
		schemaCache: ttlcache.New(
			ttlcache.WithTTL[cacheKey, *sdcpb.GetSchemaResponse](cacheConfig.TTL),
			ttlcache.WithCapacity[cacheKey, *sdcpb.GetSchemaResponse](cacheConfig.Capacity),
		),
		c: sdcpb.NewSchemaServerClient(cc),

		cacheConfig: cacheConfig,
		getOpts:     []ttlcache.Option[cacheKey, *sdcpb.GetSchemaResponse]{},
	}
	if cacheConfig.RefreshOnHit {
		rc.getOpts = []ttlcache.Option[cacheKey, *sdcpb.GetSchemaResponse]{
			ttlcache.WithDisableTouchOnHit[cacheKey, *sdcpb.GetSchemaResponse](),
		}
	}
	go rc.schemaCache.Start()
	return rc
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
	// no cache, query from the remote server.
	if c.schemaCache == nil {
		return c.c.GetSchema(ctx, in, opts...)
	}
	// if the cache has no descriptions, query from the remote server.
	if in.GetWithDescription() && !c.cacheConfig.WithDescription {
		return c.c.GetSchema(ctx, in, opts...)
	}
	// build cache entry key
	key := cacheKey{
		Name:    in.GetSchema().GetName(),
		Vendor:  in.GetSchema().GetVendor(),
		Version: in.GetSchema().GetVersion(),
		Path:    utils.ToXPath(in.GetPath(), true),
	}
	// check if the key exists in the cache
	if item := c.schemaCache.Get(key, c.getOpts...); item != nil {
		// clone it
		rsp := proto.Clone(item.Value()).(*sdcpb.GetSchemaResponse)
		// apply modifiers
		// if the request does not need the description and the
		// cache stores with description, remove it.
		if !in.GetWithDescription() && c.cacheConfig.WithDescription {
			return removeDescription(rsp), nil
		}
		return rsp, nil
	}
	// key not found in the cache, query remote server
	rsp, err := c.c.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Path:            in.GetPath(),
		Schema:          in.GetSchema(),
		ValidateKeys:    in.GetValidateKeys(),
		WithDescription: c.cacheConfig.WithDescription,
	}, opts...)
	if err != nil {
		return nil, err
	}

	// populate the cache with the retrieved schema.
	c.schemaCache.Set(key, rsp, ttlcache.DefaultTTL)
	// clone the response to return it to the client.
	rrsp := proto.Clone(rsp).(*sdcpb.GetSchemaResponse)
	// apply modifiers
	// if the request does not need the description and the
	// cache stores with description, remove it.
	if !in.GetWithDescription() && c.cacheConfig.WithDescription {
		return removeDescription(rrsp), nil
	}
	return rrsp, nil
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
	rsp, err := c.c.DeleteSchema(ctx, in, opts...)
	if err != nil {
		return nil, err
	}

	if c.schemaCache == nil {
		return rsp, nil
	}
	c.schemaCache.Range(func(item *ttlcache.Item[cacheKey, *sdcpb.GetSchemaResponse]) bool {
		if item.Key().Name != in.GetSchema().GetName() {
			return true // continue
		}
		if item.Key().Vendor != in.GetSchema().GetVendor() {
			return true // continue
		}
		if item.Key().Version != in.GetSchema().GetVersion() {
			return true // continue
		}
		c.schemaCache.Delete(item.Key())
		return true
	})
	return nil, nil
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
	if c.schemaCache == nil {
		return c.c.ToPath(ctx, in, opts...)
	}
	numPathElems := len(in.GetPathElement())
	p := &sdcpb.Path{
		Elem: make([]*sdcpb.PathElem, 0, numPathElems),
	}
	i := 0
OUTER:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if i >= numPathElems {
				break OUTER
			}
			p.Elem = append(p.Elem, &sdcpb.PathElem{Name: in.PathElement[i]})
			rsp, err := c.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:            p,
				Schema:          in.GetSchema(),
				ValidateKeys:    false,
				WithDescription: false,
			}, opts...)
			if err != nil {
				return nil, err
			}
			switch rsp.GetSchema().Schema.(type) {
			case *sdcpb.SchemaElem_Container:
				p.Elem[len(p.GetElem())-1].Key = make(map[string]string, len(rsp.GetSchema().GetContainer().GetKeys()))
				for _, schemaKey := range rsp.GetSchema().GetContainer().GetKeys() {
					p.Elem[len(p.GetElem())-1].Key[schemaKey.GetName()] = in.GetPathElement()[i+1]
					i++
					if i >= len(in.GetPathElement()) {
						break OUTER
					}
				}
			case *sdcpb.SchemaElem_Field:
			case *sdcpb.SchemaElem_Leaflist:
			}
			//
			i++
		}
	}
	if numPathElems > i {
		return nil, fmt.Errorf("unknown PathElement: %s", in.GetPathElement()[i])
	}
	return &sdcpb.ToPathResponse{Path: p}, nil
}

// ExpandPath returns a list of sub paths given a single path
func (c *remoteClient) ExpandPath(ctx context.Context, in *sdcpb.ExpandPathRequest, opts ...grpc.CallOption) (*sdcpb.ExpandPathResponse, error) {
	return c.c.ExpandPath(ctx, in, opts...)
}

// GetSchemaElements returns the schema of each path element
func (c *remoteClient) GetSchemaElements(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (chan *sdcpb.SchemaElem, error) {
	ch := make(chan *sdcpb.SchemaElem)
	if c.schemaCache == nil {
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
					log.Errorf("stream rcv ended: %v", err)
					return
				}
				ch <- msg.GetSchema()
			}
		}()
		return ch, nil
	}
	//
	go func() {
		defer close(ch)
		for _, sp := range toSubPaths(in.GetPath()) {
			r, err := c.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:            sp,
				Schema:          in.GetSchema(),
				ValidateKeys:    in.GetValidateKeys(),
				WithDescription: in.GetWithDescription(),
			}, opts...)
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					return
				}
				log.Errorf("GetSchema cache failed: %v", err)
				return
			}
			ch <- r.GetSchema()
		}
	}()
	return ch, nil
}

func removeDescription(rsp *sdcpb.GetSchemaResponse) *sdcpb.GetSchemaResponse {
	if rsp == nil {
		return nil
	}

	switch rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		rsp.GetSchema().GetContainer().Description = ""
	case *sdcpb.SchemaElem_Field:
		rsp.GetSchema().GetField().Description = ""
	case *sdcpb.SchemaElem_Leaflist:
		rsp.GetSchema().GetLeaflist().Description = ""
	}
	return rsp
}

func toSubPaths(p *sdcpb.Path) []*sdcpb.Path {
	rs := make([]*sdcpb.Path, 0, len(p.GetElem()))

	for i := range p.GetElem() {
		rs = append(rs, &sdcpb.Path{Elem: p.GetElem()[:i+1]})
	}

	return rs
}
