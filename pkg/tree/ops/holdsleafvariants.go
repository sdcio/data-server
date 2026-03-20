package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func HoldsLeafVariants(e api.Entry) bool {
	switch x := e.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		return x.Container.GetIsPresence()
	case *sdcpb.SchemaElem_Leaflist:
		return true
	case *sdcpb.SchemaElem_Field:
		return true
	}
	return false
}
