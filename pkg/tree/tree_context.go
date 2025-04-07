package tree

import (
	"fmt"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
)

type TreeContext struct {
	root         Entry // the trees root element
	schemaClient schemaClient.SchemaClientBound
	actualOwner  string
}

func NewTreeContext(sc schemaClient.SchemaClientBound, actualOwner string) *TreeContext {
	return &TreeContext{
		schemaClient: sc,
		actualOwner:  actualOwner,
	}
}

// deepCopy root is required to be set manually
func (t *TreeContext) deepCopy() *TreeContext {
	return &TreeContext{
		schemaClient: t.schemaClient,
	}
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
