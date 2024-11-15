package tree

import (
	"context"
	"strings"
	"sync"

	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type schemaIndex struct {
	index        map[string]*sdcpb.GetSchemaResponse
	indexMutex   sync.RWMutex
	ongoing      map[string]*sync.Cond
	ongoingMutex sync.Mutex
	scb          SchemaClient.SchemaClientBound
}

func newSchemaIndex(scb SchemaClient.SchemaClientBound) *schemaIndex {
	si := &schemaIndex{
		index:   map[string]*sdcpb.GetSchemaResponse{},
		ongoing: map[string]*sync.Cond{},
		scb:     scb,
	}
	return si
}

func (si *schemaIndex) retrieveCache(k string) (*sdcpb.GetSchemaResponse, bool) {
	si.indexMutex.RLock()
	defer si.indexMutex.RUnlock()
	val, exists := si.index[k]
	return val, exists
}

// retrieveOrCreateOngoingCond retrieve the sync.Cond from the map. If it does not exist, it is being created.
// the bool indicates if the sync.Cond is new or an existing entry in the map
func (si *schemaIndex) retrieveOrCreateOngoingCond(k string) (*sync.Cond, bool) {
	// lets lock the ongoing map
	si.ongoingMutex.Lock()
	defer si.ongoingMutex.Unlock()
	cond, exists := si.ongoing[k]
	// if k does not exist we need to add a Condition and query the schemaserver
	if !exists {
		cond = sync.NewCond(&sync.Mutex{})
		si.ongoing[k] = cond
	}
	return cond, exists
}

func (si *schemaIndex) Retrieve(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {

	// convert the path into a keyless path, for schema index lookups.
	keylessPathSlice := utils.ToStrings(path, false, true)
	keylessPath := strings.Join(keylessPathSlice, PATHSEP)

	val, exists := si.retrieveCache(keylessPath)
	if exists {
		return val, nil
	}
	// does not exist in index yet

	// lets lock the ongoing map
	cond, existed := si.retrieveOrCreateOngoingCond(keylessPath)

	// if it existed, some other goroutine is already fetching the schema
	if existed {
		cond.L.Lock()
		defer cond.L.Unlock()
		loop := true
		// there is already a request ongoing, lets wait for it and then grab it from the cache
		for loop {
			cond.Wait()
			val, exists = si.retrieveCache(keylessPath)
			loop = !exists
		}

		return val, nil
	}

	// if schema wasn't found in index, go and fetch it
	schemaRsp, err := si.scb.GetSchema(ctx, path)
	if err != nil {
		return nil, err
	}

	si.indexMutex.Lock()
	// store the schema in the lookup index
	si.index[keylessPath] = schemaRsp
	si.indexMutex.Unlock()
	cond.Broadcast()
	return schemaRsp, nil
}
