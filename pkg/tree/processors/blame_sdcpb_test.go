package processors_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/tree/processors"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestBlameNodeToSdcpbOmitsEmptyValues(t *testing.T) {
	e := processors.BlameNodeToSdcpb(&api.BlameNode{Name: "n", Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "v"}}, Children: []*api.BlameNode{{Name: "c"}}})
	if e.GetValue().GetStringVal() != "v" || e.GetDeviationValue() != nil {
		t.Fatalf("root = %v", e)
	}
	if len(e.GetChilds()) != 1 || e.GetChilds()[0].GetValue() != nil {
		t.Fatalf("child = %v", e.GetChilds())
	}
	if processors.BlameNodeToSdcpb(nil) != nil {
		t.Fatal("nil node must stay nil")
	}
}
