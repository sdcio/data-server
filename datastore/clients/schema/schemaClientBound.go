package schemaClient

import (
	"context"
	"errors"
	"io"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type SchemaClientBound struct {
	schema       *schemapb.Schema
	schemaClient schemapb.SchemaServerClient
}

func NewSchemaClientBound(s *schemapb.Schema, sc schemapb.SchemaServerClient) *SchemaClientBound {
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
	stream, err := scb.schemaClient.GetSchemaElements(ctx, gsr)
	if err != nil {
		return nil, err
	}
	ch := make(chan *schemapb.GetSchemaResponse)
	go func() {
		defer close(ch)
		for {
			r, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				log.Errorf("GetSchemaElements stream err: %v", err)
				return
			}
			select {
			case <-done:
				return
			case ch <- r:
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
