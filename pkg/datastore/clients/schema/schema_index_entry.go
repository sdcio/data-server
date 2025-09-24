package schemaClient

import (
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type SchemaIndexEntry struct {
	schemaRsp *sdcpb.GetSchemaResponse
	err       error
	mu        sync.Mutex
	ready     bool
}

func NewSchemaIndexEntry(schemaRsp *sdcpb.GetSchemaResponse, err error) *SchemaIndexEntry {
	sie := &SchemaIndexEntry{
		schemaRsp: schemaRsp,
		err:       err,
		mu:        sync.Mutex{},
	}
	return sie
}

func (s *SchemaIndexEntry) Lock() {
	s.mu.Lock()
}

func (s *SchemaIndexEntry) Unlock() {
	s.mu.Unlock()
}

func (s *SchemaIndexEntry) Get() (*sdcpb.GetSchemaResponse, error) {
	return s.schemaRsp, s.err
}

func (s *SchemaIndexEntry) SetSchemaResponseAndError(r *sdcpb.GetSchemaResponse, err error) {
	s.err = err
	s.schemaRsp = r
	s.ready = true
}

func (s *SchemaIndexEntry) GetError() error {
	return s.err
}

func (s *SchemaIndexEntry) GetSchemaResponse() *sdcpb.GetSchemaResponse {
	return s.schemaRsp
}

func (s *SchemaIndexEntry) GetReady() bool {
	return s.ready
}
