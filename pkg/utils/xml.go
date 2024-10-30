package utils

import (
	"slices"
	"strings"

	"github.com/beevik/etree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	NcBase1_0 = "urn:ietf:params:xml:ns:netconf:base:1.0"
)

type XMLOperation string

const (
	XMLOperationDelete  XMLOperation = "delete"
	XMLOperationRemove  XMLOperation = "remove"
	XMLOperationReplace XMLOperation = "replace"
)

func TypedValueToXML(parent *etree.Element, tv *sdcpb.TypedValue, name string, namespace string, onlyNewOrUpdated bool, operationWithNamespace bool, useOperationRemove bool) {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_LeaflistVal:
		// we add all the leaflist entries as their own values
		for _, tvle := range tv.GetLeaflistVal().GetElement() {
			TypedValueToXML(parent, tvle, name, namespace, onlyNewOrUpdated, operationWithNamespace, useOperationRemove)
		}
		if onlyNewOrUpdated {
			AddXMLOperation(parent, XMLOperationReplace, operationWithNamespace, useOperationRemove)
		}

	case *sdcpb.TypedValue_EmptyVal:
		parent.CreateElement(name)
	default:
		subelem := parent.CreateElement(name)
		if namespace != "" {
			subelem.CreateAttr("xmlns", namespace)
		}
		subelem.SetText(TypedValueToString(tv))
	}
}

// AddXMLOperation adds the operation Attribute to the given etree.Element
// if the operation is XMLOperationDelete or XMLOperationRemove, the useOperationRemove parameter defines which operation of these is finally used.
// if XMLOperationReplace is providede, the replace operation is addded.
func AddXMLOperation(elem *etree.Element, operation XMLOperation, operationWithNamespace bool, useOperationRemove bool) {
	var operName string
	switch operation {
	case XMLOperationDelete, XMLOperationRemove:
		operName = string(XMLOperationDelete)
		if useOperationRemove {
			operName = string(XMLOperationRemove)
		}
	case XMLOperationReplace:
		operName = string(XMLOperationReplace)
	}

	operKey := "operation"
	// add base1.0 as xmlns:nc attr
	if operationWithNamespace {
		elem.CreateAttr("xmlns:nc", NcBase1_0)
		operKey = "nc:" + operKey
	}
	// add the delete operation attribute
	elem.CreateAttr(operKey, string(operName))
}

// XmlRecursiveSortElementsByTagName - is a function used in testing to recursively sort XML elements by their tag name
func XmlRecursiveSortElementsByTagName(element *etree.Element) {
	// Sort the child elements by their tag name
	slices.SortStableFunc(element.Child, func(i, j etree.Token) int {
		ci, oki := i.(*etree.Element)
		cj, okj := j.(*etree.Element)

		if oki && okj {
			comp := strings.Compare(ci.Tag, cj.Tag)
			if comp != 0 {
				return comp
			}
			attributes := []string{"name", "index"}
			for _, a := range attributes {
				if cic := ci.SelectElement(a); cic != nil {
					cjc := cj.SelectElement(a)
					return strings.Compare(cic.Text(), cjc.Text())
				}
			}
		}
		return 0
	})

	// Recurse into each child element to sort their children
	for _, child := range element.Child {
		if celem, ok := child.(*etree.Element); ok {
			XmlRecursiveSortElementsByTagName(celem)
		}
	}
}
