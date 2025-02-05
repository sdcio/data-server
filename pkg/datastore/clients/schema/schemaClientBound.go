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
	"strings"
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/utils"
)

const (
	PATHSEP = "/"
)

// SchemaClientBound provides access to a certain vendor + model + version based schema
type SchemaClientBound interface {
	// GetSchema retrieves the schema for the given path
	GetSchemaSdcpbPath(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error)
	GetSchemaSlicePath(ctx context.Context, path []string) (*sdcpb.GetSchemaResponse, error)
	// GetSchemaElements retrieves the Schema Elements for all levels of the given path
	GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error)
	ToPath(ctx context.Context, path []string) (*sdcpb.Path, error)
}

type SchemaClientBoundImpl struct {
	schema       *sdcpb.Schema
	schemaClient schema.Client

	index      sync.Map // string -> schemaIndexEntry
	indexMutex sync.RWMutex
}

func NewSchemaClientBound(s *sdcpb.Schema, sc schema.Client) *SchemaClientBoundImpl {
	result := &SchemaClientBoundImpl{
		schema:       s,
		schemaClient: sc,
		index:        sync.Map{},
	}
	return result
}

// GetSchema retrieves the schema for the given path
func (scb *SchemaClientBoundImpl) GetSchemaSdcpbPath(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	return scb.Retrieve(ctx, path)
}

// GetSchema retrieves the given schema element from the schema-server.
// relies on TreeSchemaCacheClientImpl.retrieveSchema(...) to source the internal lookup index (cache) of schemas
func (scb *SchemaClientBoundImpl) GetSchemaSlicePath(ctx context.Context, path []string) (*sdcpb.GetSchemaResponse, error) {
	// convert the []string path into sdcpb.path for schema retrieval
	sdcpbPath, err := scb.ToPath(ctx, path)
	if err != nil {
		return nil, err
	}

	return scb.Retrieve(ctx, sdcpbPath)
}

func (scb *SchemaClientBoundImpl) Retrieve(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	// convert the path into a keyless path, for schema index lookups.
	keylessPathSlice := utils.ToStrings(path, false, true)
	keylessPath := strings.Join(keylessPathSlice, PATHSEP)

	entryAny, loaded := scb.index.LoadOrStore(keylessPath, NewSchemaIndexEntry(nil, nil))
	entry := entryAny.(*schemaIndexEntry)

	// Lock the entry to prevent race conditions
	entry.mu.Lock()
	defer entry.mu.Unlock()

	// if it existed, some other goroutine is already fetching the schema
	if loaded && entry.ready {
		return entry.schemaRsp, entry.err
	}

	// retrieve Schema via schemaclient
	schema, err := scb.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Schema:          scb.getSchema(),
		Path:            path,
		WithDescription: false,
	})
	entry.schemaRsp = schema
	entry.err = err
	entry.ready = true

	return entry.Get()
}

// ToPath local implementation of the ToPath functinality. It takes a string slice that contains schema elements as well as key values.
// Via the help of the schema, the key elemens are being identified and an sdcpb.Path is returned.
func (scb *SchemaClientBoundImpl) ToPath(ctx context.Context, path []string) (*sdcpb.Path, error) {
	p := &sdcpb.Path{}
	// iterate through the path slice
	for i := 0; i < len(path); i++ {
		// create a PathElem for the actual index
		newPathElem := &sdcpb.PathElem{Name: path[i]}
		// append the path elem to the path
		p.Elem = append(p.Elem, newPathElem)
		// retrieve the schema
		schema, err := scb.Retrieve(ctx, p)
		if err != nil {
			return nil, err
		}

		// break early if the container itself is defined in the path, not a sub-element
		if len(path) <= i+1 {
			break
		}

		// if it is a container with keys
		if schemaKeys := schema.GetSchema().GetContainer().GetKeys(); schemaKeys != nil {
			// add key map
			newPathElem.Key = make(map[string]string, len(schemaKeys))
			// adding the keys with the value from path[i], which is the key value
			for _, k := range schemaKeys {
				i++
				newPathElem.Key[k.Name] = path[i]
			}
		}
	}
	return p, nil
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
