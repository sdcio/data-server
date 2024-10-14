package tree

import (
	"fmt"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

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
		// the deletion case
		if !s.remainsToExist() {
			utils.AddXMLOperation(parent, operationWithNamespace, useOperationRemove)
			parentSchema, levelsUp := s.GetFirstAncestorWithSchema()
			schemaKeys := parentSchema.GetSchemaKeys()
			var treeElem Entry = s
			for i := levelsUp - 1; i >= 0; i-- {
				parent.CreateElement(schemaKeys[i]).SetText(treeElem.PathName())
				treeElem = treeElem.GetParent()
			}
			return true, nil
		}

		overAllDoAdd := false
		for _, c := range s.filterActiveChoiceCaseChilds() {
			// recurse the call
			doAdd, err := c.toXmlInternal(parent, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
			if err != nil {
				return false, err
			}
			overAllDoAdd = doAdd || overAllDoAdd
		}
		return overAllDoAdd, nil
	case *sdcpb.SchemaElem_Container:
		if !s.remainsToExist() {
			e := parent.CreateElement(s.pathElemName)
			if honorNamespace && !namespaceIsEqual(s, s.parent) {
				e.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(s.GetSchema()))
			}
			utils.AddXMLOperation(e, operationWithNamespace, useOperationRemove)
			return true, nil
		}
		overAllDoAdd := false
		if len(s.GetSchemaKeys()) > 0 {
			// if the container contains keys, then it is a list
			// hence must be rendered as an array
			childs, err := s.FilterChilds(nil)
			if err != nil {
				return false, err
			}

			for _, c := range childs {
				p := etree.NewElement(s.PathName())
				if honorNamespace && !namespaceIsEqual(s, s.parent) {
					p.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(s.GetSchema()))
				}
				doAdd, err := c.toXmlInternal(p, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
				if err != nil {
					return false, err
				}
				overAllDoAdd = doAdd || overAllDoAdd
				if doAdd {
					parent.AddChild(p)
				}
			}
			return overAllDoAdd, nil
		}

		p := etree.NewElement(s.PathName())
		for _, c := range s.childs {
			if s.parent != nil {
				if honorNamespace && !namespaceIsEqual(s, s.parent) {
					p.CreateAttr("xmlns", utils.GetNamespaceFromGetSchema(s.GetSchema()))
				}
			} else {
				p = parent
			}
			doAdd, err := c.toXmlInternal(p, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
			if err != nil {
				return false, err
			}
			overAllDoAdd = doAdd || overAllDoAdd
		}
		if overAllDoAdd && s.parent != nil {
			parent.AddChild(p)
		}
		return overAllDoAdd, nil
	case *sdcpb.SchemaElem_Leaflist, *sdcpb.SchemaElem_Field:
		if !s.remainsToExist() {
			utils.AddXMLOperation(parent.CreateElement(s.pathElemName), operationWithNamespace, useOperationRemove)
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
		le := s.leafVariants.GetHighestPrecedence(false)
		if onlyNewOrUpdated && !(le.IsNew || le.IsUpdated) {
			return false, nil
		}
		v, err := le.Update.Value()
		if err != nil {
			return false, err
		}
		ns := ""
		if honorNamespace && !namespaceIsEqual(s, s.parent) {
			ns = utils.GetNamespaceFromGetSchema(s.GetSchema())
		}
		utils.TypedValueToXML(parent, v, s.PathName(), ns, operationWithNamespace, useOperationRemove)

		return true, nil
	}
	return false, fmt.Errorf("unable to convert to xml (%s)", s.Path())
}

// namespaceIsEqual takes the two given Entries, gets the namespace
// and reports if both belong to the same namespace
func namespaceIsEqual(a Entry, b Entry) bool {
	aSchema := a.GetSchema()
	bSchema := b.GetSchema()

	if aSchema == nil {
		aAncest, _ := a.GetFirstAncestorWithSchema()
		aSchema = aAncest.GetSchema()
	}
	if bSchema == nil {
		bAncest, _ := b.GetFirstAncestorWithSchema()
		bSchema = bAncest.GetSchema()
	}
	return utils.GetNamespaceFromGetSchema(aSchema) == utils.GetNamespaceFromGetSchema(bSchema)
}
