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

type SchemaClientBound struct {
	schema       *sdcpb.Schema
	schemaClient schema.Client
}

func NewSchemaClientBound(s *sdcpb.Schema, sc schema.Client) *SchemaClientBound {
	return &SchemaClientBound{
		schema:       s,
		schemaClient: sc,
	}
}

// GetSchema retrieves the schema for the given path
func (scb *SchemaClientBound) GetSchema(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	return scb.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Schema: scb.getSchema(),
		Path:   path,
	})
}

func (scb *SchemaClientBound) GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error) {
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

func (scb *SchemaClientBound) getSchema() *sdcpb.Schema {
	return &sdcpb.Schema{
		Name:    scb.schema.Name,
		Version: scb.schema.Version,
		Vendor:  scb.schema.Vendor,
	}
}
