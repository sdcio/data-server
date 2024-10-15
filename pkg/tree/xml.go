package tree

import (
	"fmt"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ToXML yields the xml representation of the tree. Either updates only (onlyNewOrUpdated flag) or the actual view on the whole tree.
// If honorNamespace is set, the xml elements will carry their respective namespace attributes.
// If operationWithNamespace is set, the operation attributes added to the to be deleted alements will also carry the Netconf Base namespace.
// If useOperationRemove is set, the remove operation will be used for deletes, instead of the delete operation.
func (s *sharedEntryAttributes) ToXML(onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove bool) (*etree.Document, error) {
	doc := etree.NewDocument()
	_, err := s.toXmlInternal(&doc.Element, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *sharedEntryAttributes) toXmlInternal(parent *etree.Element, onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (doAdd bool, err error) {

	switch s.schema.GetSchema().(type) {
	case nil:
		// This case represents a key level element. So no schema present. all child attributes need to be adedd directly to the parent element, since the key levels are not visible in the resulting xml.
		if !s.remainsToExist() {
			// If the element is to be deleted
			// add the delete operation to the parent element
			utils.AddXMLOperationRemoveDelete(parent, operationWithNamespace, useOperationRemove)
			// retrieve the parent schema, we need to extract the key names
			// values are the tree level names
			parentSchema, levelsUp := s.GetFirstAncestorWithSchema()
			// from the parent we get the keys as slice
			schemaKeys := parentSchema.GetSchemaKeys()
			var treeElem Entry = s
			// the keys do match the levels up in the tree in reverse order
			// hence we init i with levelUp and count down
			for i := levelsUp - 1; i >= 0; i-- {
				// and finally we create the patheleme key attributes
				parent.CreateElement(schemaKeys[i]).SetText(treeElem.PathName())
				treeElem = treeElem.GetParent()
			}
			return true, nil
		}

		// if the entry remains so exist, we need to add it to the xml doc
		overAllDoAdd := false
		for _, c := range s.filterActiveChoiceCaseChilds() {
			// recurse the call
			// no additional element is created, since we're on a key level, so add to parent element
			doAdd, err := c.toXmlInternal(parent, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
			if err != nil {
				return false, err
			}
			// only if there was something added in the childs, the element itself is meant to be added.
			// we keep track of that via overAllDoAdd.
			overAllDoAdd = doAdd || overAllDoAdd
		}
		return overAllDoAdd, nil
	case *sdcpb.SchemaElem_Container:
		// check if the element is meant to be deleted
		if !s.remainsToExist() {
			// if delete, create the element as child of parent
			e := parent.CreateElement(s.pathElemName)
			// add namespace if we create doc with namespace and the actual namespace differs from the parent namespace
			if honorNamespace && !namespaceIsEqual(s, s.parent) {
				// create namespace attribute
				e.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(s.GetSchema()))
			}
			// add the delete / remove operation
			utils.AddXMLOperationRemoveDelete(e, operationWithNamespace, useOperationRemove)
			return true, nil
		}
		overAllDoAdd := false

		// A container can represent a list or a map.
		// Figure out if it has keys, then it is a map
		if len(s.GetSchemaKeys()) > 0 {
			// if the container contains keys, then it is a list
			// hence must be rendered as an array
			childs, err := s.FilterChilds(nil)
			if err != nil {
				return false, err
			}
			// go through the childs creating the xml elements
			for _, c := range childs {
				// create the element for the child, that in the recursed call will appear as parent
				p := etree.NewElement(s.PathName())
				// process the honorNamespace instruction
				if honorNamespace && !namespaceIsEqual(s, s.parent) {
					p.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(s.GetSchema()))
				}
				// recurse the call
				doAdd, err := c.toXmlInternal(p, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
				if err != nil {
					return false, err
				}
				overAllDoAdd = doAdd || overAllDoAdd
				// add the child only if doAdd is true
				if doAdd {
					parent.AddChild(p)
				}
			}
			return overAllDoAdd, nil
		}
		// If we reach here, the Tree Entry represents a list
		// So create the element that the tree entry represents
		p := etree.NewElement(s.PathName())
		// iterate through all the childs
		for _, c := range s.childs {
			// for namespace attr creation we need to handle the root node (s.parent == nil) specially
			if s.parent != nil {
				// only if not the root level, we can check if parent namespace != actual elements namespace
				// so if we need to add namespaces, check if they are equal, if not add the namespace attribute
				if honorNamespace && !namespaceIsEqual(s, s.parent) {
					p.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(s.GetSchema()))
				}
			} else {
				// if this is the root node, we take the given element from the parent parameter as p
				// avoiding wrongly adding an additional level in the xml doc.
				p = parent
			}
			// recurse the call to all the children
			doAdd, err := c.toXmlInternal(p, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
			if err != nil {
				return false, err
			}
			// if a branch, represented by the childs is not meant to be added the doAdd is false.
			// if all the childs are meant to no be added, the whole container element should not be added
			// so we keep track via overAllDoAdd
			overAllDoAdd = doAdd || overAllDoAdd
		}
		// so if there is at least a child and the s.parent is not nil (root node)
		// then add p to the parent as a child
		if overAllDoAdd && s.parent != nil {
			parent.AddChild(p)
		}
		return overAllDoAdd, nil
	case *sdcpb.SchemaElem_Leaflist, *sdcpb.SchemaElem_Field:
		// check if the element remains to exist
		if !s.remainsToExist() {
			// if not, add the remove / delete op
			utils.AddXMLOperationRemoveDelete(parent.CreateElement(s.pathElemName), operationWithNamespace, useOperationRemove)
			// see case nil for an explanation of this, it is basically the same
			if s.parent.GetSchema() == nil {
				// we need to add the keys
				parentSchema, levelsUp := s.parent.GetFirstAncestorWithSchema()
				schemaKeys := parentSchema.GetSchemaKeys()
				var treeElem Entry = s.parent
				for i := levelsUp - 1; i >= 0; i-- {
					parent.CreateElement(schemaKeys[i]).SetText(treeElem.PathName())
					treeElem = treeElem.GetParent()
				}
			}
			return true, nil
		}
		// if the Field or Leaflist remains to exist
		// get highes Precedence value
		le := s.leafVariants.GetHighestPrecedence(false)
		// check the only new or updated flag
		if onlyNewOrUpdated && !(le.IsNew || le.IsUpdated) {
			return false, nil
		}
		v, err := le.Update.Value()
		if err != nil {
			return false, err
		}
		ns := ""
		// process the namespace attribute
		if honorNamespace && !namespaceIsEqual(s, s.parent) {
			ns = utils.GetNamespaceFromGetSchema(s.GetSchema())
		}
		// convert value to XML and add to parent
		utils.TypedValueToXML(parent, v, s.PathName(), ns, operationWithNamespace, useOperationRemove)
		return true, nil
	}
	return false, fmt.Errorf("unable to convert to xml (%s)", s.Path())
}

// namespaceIsEqual takes the two given Entries, gets the namespace
// and reports if both belong to the same namespace
func namespaceIsEqual(a Entry, b Entry) bool {
	// store for the calculated namespaces
	namespaces := make([]string, 0, 2)
	for _, e := range []Entry{a, b} {
		// get schemas for a and b
		schema := e.GetSchema()

		// if schema is nil, we're in a key level in the tree, so search up the chain for
		// the first ancestor that contains a schema.
		if schema == nil {
			ancest, _ := a.GetFirstAncestorWithSchema()
			schema = ancest.GetSchema()
		}
		// add the namespace to the array
		namespaces = append(namespaces, utils.GetNamespaceFromGetSchema(schema))
	}

	// compare the two namespaces
	return namespaces[0] == namespaces[1]
}
