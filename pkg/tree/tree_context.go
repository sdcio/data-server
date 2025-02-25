package tree

import (
	"fmt"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
)

type TreeContext struct {
	root         Entry // the trees root element
	cacheClient  TreeCacheClient
	schemaClient schemaClient.SchemaClientBound
	actualOwner  string
}

func NewTreeContext(cc TreeCacheClient, sc schemaClient.SchemaClientBound, actualOwner string) *TreeContext {
	return &TreeContext{
		cacheClient:  cc,
		schemaClient: sc,
		actualOwner:  actualOwner,
	}
}

// deepCopy root is required to be set manually
func (t *TreeContext) deepCopy() *TreeContext {
	return &TreeContext{
		cacheClient: t.cacheClient,
	}
}

func (t *TreeContext) GetTreeCacheClient() TreeCacheClient {
	return t.cacheClient
}

func (t *TreeContext) SetRoot(e Entry) error {
	if t.root != nil {
		return fmt.Errorf("trying to set treecontexts root, although it is already set")
	}
	t.root = e
	return nil
}

func (t *TreeContext) GetActualOwner() string {
	return t.actualOwner
}

func (t *TreeContext) SetActualOwner(owner string) {
	t.actualOwner = owner
}
