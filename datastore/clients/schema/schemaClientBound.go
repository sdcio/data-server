package schemaClient

import (
	"context"

	"github.com/iptecharch/data-server/schema"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

type SchemaClientBound struct {
	schema       *schemapb.Schema
	schemaClient schema.Client
}

func NewSchemaClientBound(s *schemapb.Schema, sc schema.Client) *SchemaClientBound {
	return &SchemaClientBound{
		schema:       s,
		schemaClient: sc,
	}
}

// GetSchema retrieves the schema for the given path
func (scb *SchemaClientBound) GetSchema(ctx context.Context, path *schemapb.Path) (*schemapb.GetSchemaResponse, error) {
	return scb.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Schema: scb.getSchema(),
		Path:   path,
	})
}

func (scb *SchemaClientBound) GetSchemaElements(ctx context.Context, p *schemapb.Path, done chan struct{}) (chan *schemapb.GetSchemaResponse, error) {
	gsr := &schemapb.GetSchemaRequest{
		Path:   p,
		Schema: scb.getSchema(),
	}
	och, err := scb.schemaClient.GetSchemaElements(ctx, gsr)
	if err != nil {
		return nil, err
	}
	ch := make(chan *schemapb.GetSchemaResponse)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case se, ok := <-och:
				if !ok {
					return
				}
				ch <- &schemapb.GetSchemaResponse{
					Schema: se,
				}
			}
		}
	}()
	return ch, nil
}

func (scb *SchemaClientBound) getSchema() *schemapb.Schema {
	return &schemapb.Schema{
		Name:    scb.schema.Name,
		Version: scb.schema.Version,
		Vendor:  scb.schema.Vendor,
	}
}
