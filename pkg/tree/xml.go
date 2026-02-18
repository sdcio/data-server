package tree

import (
	"cmp"
	"fmt"
	"slices"
	"sort"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ToXML yields the xml representation of the tree. Either updates only (onlyNewOrUpdated flag) or the actual view on the whole tree.
// If honorNamespace is set, the xml elements will carry their respective namespace attributes.
// If operationWithNamespace is set, the operation attributes added to the to be deleted alements will also carry the Netconf Base namespace.
// If useOperationRemove is set, the remove operation will be used for deletes, instead of the delete operation.
func (s *sharedEntryAttributes) ToXML(onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove bool) (*etree.Document, error) {
	doc := etree.NewDocument()
	_, err := s.ToXmlInternal(&doc.Element, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *sharedEntryAttributes) ToXmlInternal(parent *etree.Element, onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (doAdd bool, err error) {

	switch s.schema.GetSchema().(type) {
	case nil:
		// This case represents a key level element. So no schema present. all child attributes need to be adedd directly to the parent element, since the key levels are not visible in the resulting xml.
		if s.ShouldDelete() {
			// If the element is to be deleted
			// add the delete operation to the parent element
			utils.AddXMLOperation(parent, utils.XMLOperationDelete, operationWithNamespace, useOperationRemove)
			// retrieve the parent schema, we need to extract the key names
			// values are the tree level names
			xmlAddKeyElements(s, parent)
			return true, nil
		}

		// if the entry remains so exist, we need to add it to the xml doc
		overallDoAdd := false

		childs := s.GetChilds(types.DescendMethodActiveChilds)

		keys := make([]string, 0, len(childs))
		for k := range childs {
			keys = append(keys, k)
		}

		// Perform ordering of attributes
		schemaParent, _ := s.GetFirstAncestorWithSchema()
		if schemaParent == nil {
			return false, fmt.Errorf("no ancestor has schema for %v", s)
		}
		schemaKeys := schemaParent.GetSchemaKeys()
		slices.SortFunc(keys, func(a, b string) int {
			aIdx := slices.Index(schemaKeys, a)
			bIdx := slices.Index(schemaKeys, b)
			switch {
			case aIdx == -1 && bIdx == -1:
				// if neither are keys, sort them against each other
				return cmp.Compare(a, b)
			case aIdx == -1:
				return 1
			case bIdx == -1:
				return -1
			default:
				return cmp.Compare(aIdx, bIdx)
			}
		})

		// go through the ordered list of attributes and create the child elements
		for _, k := range keys {
			// recurse the call
			// no additional element is created, since we're on a key level, so add to parent element
			doAdd, err := childs[k].ToXmlInternal(parent, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
			if err != nil {
				return false, err
			}
			// only if there was something added in the childs, the element itself is meant to be added.
			// we keep track of that via overAllDoAdd.
			overallDoAdd = doAdd || overallDoAdd
		}
		return overallDoAdd, nil
	case *sdcpb.SchemaElem_Container:
		overallDoAdd := false
		switch {
		case len(s.GetSchemaKeys()) > 0:
			// the container represents a list
			// if the container contains keys, then it is a list
			// hence must be rendered as an array
			childs, err := s.FilterChilds(nil)
			if err != nil {
				return false, err
			}

			// Apply sorting
			slices.SortFunc(childs, getListEntrySortFunc(s))

			// go through the childs creating the xml elements
			for _, child := range childs {
				// create the element for the child, that in the recursed call will appear as parent
				newElem := etree.NewElement(s.PathName())
				// process the honorNamespace instruction
				xmlAddNamespaceConditional(s, s.parent, newElem, honorNamespace)
				// recurse the call
				doAdd, err := child.ToXmlInternal(newElem, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
				if err != nil {
					return false, err
				}
				overallDoAdd = doAdd || overallDoAdd
				// add the child only if doAdd is true
				if doAdd {
					// Are we performing a replace?
					replaceAttr := newElem.SelectAttr("nc:operation")
					if replaceAttr != nil && replaceAttr.Value == string(utils.XMLOperationReplace) {
						// If we are replacing, we need to have all child values added to the xml tree else they will be removed from the device
						err := xmlAddAllChildValues(child, newElem, honorNamespace, operationWithNamespace, useOperationRemove)
						if err != nil {
							return false, err
						}
					} else {
						// if we are not performing a replace, add all the key elements if they do not already exist
						xmlAddKeyElements(child, newElem)
					}
					parent.AddChild(newElem)
				}
			}
			return overallDoAdd, nil
		case s.ShouldDelete():
			// s is meant to be removed
			// if delete, create the element as child of parent
			newElem := parent.CreateElement(s.pathElemName)
			// add namespace if we create doc with namespace and the actual namespace differs from the parent namespace
			xmlAddNamespaceConditional(s, s.parent, newElem, honorNamespace)
			// add the delete / remove operation
			utils.AddXMLOperation(newElem, utils.XMLOperationDelete, operationWithNamespace, useOperationRemove)
			return true, nil
		case s.GetSchema().GetContainer().IsPresence && s.containsOnlyDefaults():
			// process presence containers with no childs
			if onlyNewOrUpdated {
				// presence containers have leafvariantes with typedValue_Empty, so check that
				if s.leafVariants.ShouldDelete() {
					return false, nil
				}
				le := s.leafVariants.GetHighestPrecedence(false, false, false)
				if le == nil || onlyNewOrUpdated && !(le.IsNew || le.IsUpdated) {
					return false, nil
				}
			}
			newElem := parent.CreateElement(s.PathName())
			// process the honorNamespace instruction
			xmlAddNamespaceConditional(s, s.parent, newElem, honorNamespace)
			return true, nil

		default:
			// the container represents a map
			// So create the element that the tree entry represents
			newElem := etree.NewElement(s.PathName())

			// Apply sorting of childs
			keys := s.childs.GetKeys()
			if s.parent == nil {
				slices.Sort(keys)
			} else {
				cldrn := s.schema.GetContainer().GetChildren()
				slices.SortFunc(keys, func(a, b string) int {
					return cmp.Compare(slices.Index(cldrn, a), slices.Index(cldrn, b))
				})
			}

			// iterate through all the childs
			for _, k := range keys {

				// for namespace attr creation we need to handle the root node (s.parent == nil) specially
				if s.parent != nil {
					// only if not the root level, we can check if parent namespace != actual elements namespace
					// so if we need to add namespaces, check if they are equal, if not add the namespace attribute
					xmlAddNamespaceConditional(s, s.parent, newElem, honorNamespace)
				} else {
					// if this is the root node, we take the given element from the parent parameter as p
					// avoiding wrongly adding an additional level in the xml doc.
					newElem = parent
				}
				// recurse the call to all the children
				child, exists := s.childs.GetEntry(k)
				if !exists {
					return false, fmt.Errorf("child %s does not exist for %s", k, s.SdcpbPath().ToXPath(false))
				}
				// TODO: Do we also need to xmlAddAllChildValues here too?
				doAdd, err := child.ToXmlInternal(newElem, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
				if err != nil {
					return false, err
				}
				// if a branch, represented by the childs is not meant to be added the doAdd is false.
				// if all the childs are meant to no be added, the whole container element should not be added
				// so we keep track via overAllDoAdd
				overallDoAdd = doAdd || overallDoAdd
			}
			// so if there is at least a child and the s.parent is not nil (root node)
			// then add p to the parent as a child
			if overallDoAdd && s.parent != nil {
				parent.AddChild(newElem)
			}
			return overallDoAdd, nil
		}

	case *sdcpb.SchemaElem_Leaflist, *sdcpb.SchemaElem_Field:
		// check if the element remains to exist
		if s.ShouldDelete() {
			// if not, add the remove / delete op
			utils.AddXMLOperation(parent.CreateElement(s.pathElemName), utils.XMLOperationDelete, operationWithNamespace, useOperationRemove)
			// see case nil for an explanation of this, it is basically the same
			if s.parent.GetSchema() == nil {
				xmlAddKeyElements(s.parent, parent)
			}
			return true, nil
		}
		// if the Field or Leaflist remains to exist
		// get highes Precedence value
		le := s.leafVariants.GetHighestPrecedence(onlyNewOrUpdated, false, false)
		if le == nil {
			return false, nil
		}
		ns := ""
		// process the namespace attribute
		if s.parent == nil || (honorNamespace && !namespaceIsEqual(s, s.parent)) {
			ns = utils.GetNamespaceFromGetSchema(s.GetSchema())
		}
		// convert value to XML and add to parent
		utils.TypedValueToXML(parent, le.Value(), s.PathName(), ns, onlyNewOrUpdated, operationWithNamespace, useOperationRemove)
		return true, nil
	}
	return false, fmt.Errorf("unable to convert to xml (%s)", s.SdcpbPath().ToXPath(false))
}

// namespaceIsEqual takes the two given Entries, gets the namespace
// and reports if both belong to the same namespace
func namespaceIsEqual(a api.Entry, b api.Entry) bool {
	// store for the calculated namespaces
	namespaces := make([]string, 0, 2)
	for _, e := range []api.Entry{a, b} {
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

// xmlAddNamespaceConditional adds the namespace of a to elem if namespaces of a and b are different
func xmlAddNamespaceConditional(a api.Entry, b api.Entry, elem *etree.Element, honorNamespace bool) {
	if honorNamespace && (b == nil || !namespaceIsEqual(a, b)) {
		elem.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(a.GetSchema()))
	}
}

// xmlAddKeyElements determines the keys of a certain Entry in the tree and adds those to the
// element if they do not already exist.
func xmlAddKeyElements(s api.Entry, parent *etree.Element) {
	// retrieve the parent schema, we need to extract the key names
	// values are the tree level names
	parentSchema, levelsUp := s.GetFirstAncestorWithSchema()

	// from the parent we get the keys as slice
	schemaKeys := parentSchema.GetSchemaKeys()
	//issue #364: sort the slice
	sort.Strings(schemaKeys)

	var treeElem api.Entry = s
	// the keys do match the levels up in the tree in reverse order
	// hence we init i with levelUp and count down
	for i := levelsUp - 1; i >= 0; i-- {
		// skip if the element already exists
		existingElem := parent.SelectElement(schemaKeys[i])
		if existingElem == nil {
			// and finally we create the key elements in schema order
			keyElem := etree.NewElement(schemaKeys[i])
			keyElem.SetText(treeElem.PathName())
			parent.InsertChildAt(0, keyElem) // we go backwards, so always add to front of parent
			treeElem = treeElem.GetParent()
		}
	}
}

func xmlAddAllChildValues(s api.Entry, parent *etree.Element, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) error {
	parent.Child = make([]etree.Token, 0)
	_, err := s.ToXmlInternal(parent, false, honorNamespace, operationWithNamespace, useOperationRemove)
	if err != nil {
		return err
	}
	return nil
}
