package schemaClient

import (
	"context"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"

	"github.com/iptecharch/data-server/pkg/schema"
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
