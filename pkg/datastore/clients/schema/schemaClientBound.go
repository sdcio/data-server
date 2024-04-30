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

package schemaClient

import (
	"context"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/schema"
)

// SchemaClientBound provides access to a certain vendor + model + version based schema
type SchemaClientBound interface {
	// GetSchema retrieves the schema for the given path
	GetSchema(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error)
	// GetSchemaElements retrieves the Schema Elements for all levels of the given path
	GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error)
	ToPath(ctx context.Context, path []string) (*sdcpb.Path, error)
}

type SchemaClientBoundImpl struct {
	schema       *sdcpb.Schema
	schemaClient schema.Client
}

func NewSchemaClientBound(s *sdcpb.Schema, sc schema.Client) *SchemaClientBoundImpl {
	return &SchemaClientBoundImpl{
		schema:       s,
		schemaClient: sc,
	}
}

// GetSchema retrieves the schema for the given path
func (scb *SchemaClientBoundImpl) GetSchema(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	return scb.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Schema: scb.getSchema(),
		Path:   path,
	})
}

func (scb *SchemaClientBoundImpl) ToPath(ctx context.Context, path []string) (*sdcpb.Path, error) {
	tpr, err := scb.schemaClient.ToPath(ctx, &sdcpb.ToPathRequest{
		PathElement: path,
		Schema:      scb.getSchema(),
	})
	if err != nil {
		return nil, err
	}
	return tpr.GetPath(), nil
}

func (scb *SchemaClientBoundImpl) GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error) {
	gsr := &sdcpb.GetSchemaRequest{
		Path:   p,
		Schema: scb.getSchema(),
	}
	och, err := scb.schemaClient.GetSchemaElements(ctx, gsr)
	if err != nil {
		return nil, err
	}
	ch := make(chan *sdcpb.GetSchemaResponse)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case se, ok := <-och:
				if !ok {
					return
				}
				ch <- &sdcpb.GetSchemaResponse{
					Schema: se,
				}
			}
		}
	}()
	return ch, nil
}

func (scb *SchemaClientBoundImpl) getSchema() *sdcpb.Schema {
	return &sdcpb.Schema{
		Name:    scb.schema.Name,
		Version: scb.schema.Version,
		Vendor:  scb.schema.Vendor,
	}
}
