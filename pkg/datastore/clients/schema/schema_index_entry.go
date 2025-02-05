package schemaClient

import (
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type schemaIndexEntry struct {
	schemaRsp *sdcpb.GetSchemaResponse
	err       error
	mu        sync.Mutex
	ready     bool
}

func NewSchemaIndexEntry(schemaRsp *sdcpb.GetSchemaResponse, err error) *schemaIndexEntry {
	sie := &schemaIndexEntry{
		schemaRsp: schemaRsp,
		err:       err,
		mu:        sync.Mutex{},
	}
	return sie
}

func (s *schemaIndexEntry) Get() (*sdcpb.GetSchemaResponse, error) {
	return s.schemaRsp, s.err
}
