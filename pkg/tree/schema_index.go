package tree

import (
	"context"
	"strings"
	"sync"

	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
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

type schemaIndex struct {
	index      sync.Map // string -> schemaIndexEntry
	indexMutex sync.RWMutex
	scb        SchemaClient.SchemaClientBound
}

func newSchemaIndex(scb SchemaClient.SchemaClientBound) *schemaIndex {
	si := &schemaIndex{
		index: sync.Map{},
		scb:   scb,
	}
	return si
}

func (si *schemaIndex) Retrieve(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	// convert the path into a keyless path, for schema index lookups.
	keylessPathSlice := utils.ToStrings(path, false, true)
	keylessPath := strings.Join(keylessPathSlice, PATHSEP)

	entryAny, loaded := si.index.LoadOrStore(keylessPath, NewSchemaIndexEntry(nil, nil))
	entry := entryAny.(*schemaIndexEntry)

	// Lock the entry to prevent race conditions
	entry.mu.Lock()
	defer entry.mu.Unlock()

	// if it existed, some other goroutine is already fetching the schema
	if loaded && entry.ready {
		return entry.schemaRsp, entry.err
	}

	schema, err := si.scb.GetSchema(ctx, path)
	entry.schemaRsp = schema
	entry.err = err
	entry.ready = true

	return entry.Get()
}
