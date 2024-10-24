package utils

import (
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

func TypedValueToXML(parent *etree.Element, tv *sdcpb.TypedValue, name string, namespace string, operationWithNamespace bool, useOperationRemove bool) {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_LeaflistVal:
		// we add all the leaflist entries as their own values
		for _, tvle := range tv.GetLeaflistVal().GetElement() {
			TypedValueToXML(parent, tvle, name, namespace, operationWithNamespace, useOperationRemove)
		}
		AddXMLOperation(parent, XMLOperationReplace, operationWithNamespace, useOperationRemove)

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
