package processors

import (
	pubapi "github.com/sdcio/data-server/pkg/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// BlameNodeToSdcpb converts a blame tree to its sdcpb wire form. Values are
// shared, not copied.
func BlameNodeToSdcpb(n *pubapi.BlameNode) *sdcpb.BlameTreeElement {
	if n == nil {
		return nil
	}
	e := &sdcpb.BlameTreeElement{
		Name:           n.Name,
		Owner:          n.Owner,
		Value:          n.Value,
		DeviationValue: n.DeviationValue,
		KeyName:        n.KeyName,
	}
	for _, ch := range n.Children {
		e.Childs = append(e.Childs, BlameNodeToSdcpb(ch))
	}
	return e
}
