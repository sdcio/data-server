package tree

import (
	"fmt"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
)

type TreeContext struct {
	root             Entry // the trees root element
	schemaClient     schemaClient.SchemaClientBound
	actualOwner      string
	nonRevertiveInfo map[string]bool
}

func NewTreeContext(sc schemaClient.SchemaClientBound, actualOwner string) *TreeContext {
	return &TreeContext{
		schemaClient:     sc,
		actualOwner:      actualOwner,
		nonRevertiveInfo: map[string]bool{},
	}
}

// deepCopy root is required to be set manually
func (t *TreeContext) deepCopy() *TreeContext {
	tc := &TreeContext{
		schemaClient: t.schemaClient,
	}

	// deepcopy nonRevertiveInfo
	m := map[string]bool{}
	for k, v := range t.nonRevertiveInfo {
		m[k] = v
	}
	tc.nonRevertiveInfo = m
	return tc
}

func (t *TreeContext) AddNonRevertiveInfo(intent string, nonRevertive bool) {
	t.nonRevertiveInfo[intent] = nonRevertive
}

// IsNonRevertiveIntent returns the non-revertive flag per intent. False is also returned the intent does not exist.
func (t *TreeContext) IsNonRevertiveIntent(intent string) bool {
	return t.nonRevertiveInfo[intent]
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
