package utils

import (
	"github.com/beevik/etree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	NcBase1_0 = "urn:ietf:params:xml:ns:netconf:base:1.0"

	OperationDelete  = "delete"
	OperationRemove  = "remove"
	OperationReplace = "replace"
)

func TypedValueToXML(parent *etree.Element, tv *sdcpb.TypedValue, name string, namespace string, operationWithNamespace bool, useOperationRemove bool) {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_LeaflistVal:
		// we add all the leaflist entries as their own values
		for _, tvle := range tv.GetLeaflistVal().GetElement() {
			TypedValueToXML(parent, tvle, name, namespace, operationWithNamespace, useOperationRemove)
		}
		AddXMLOperation(parent, operationWithNamespace, useOperationRemove)

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
func AddXMLOperation(elem *etree.Element, operationWithNamespace bool, useOperationRemove bool) {
	operName := OperationDelete
	operKey := "operation"
	if useOperationRemove {
		operName = OperationRemove
	}
	// add base1.0 as xmlns:nc attr
	if operationWithNamespace {
		elem.CreateAttr("xmlns:nc", NcBase1_0)
		operKey = "nc:" + operKey
	}
	// add the delete operation attribute
	elem.CreateAttr(operKey, operName)
}
