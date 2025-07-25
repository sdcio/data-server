package tree

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func DefaultValueExists(schema *sdcpb.SchemaElem) bool {
	switch schem := schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Field:
		return schem.Field.GetDefault() != ""
	case *sdcpb.SchemaElem_Leaflist:
		return len(schem.Leaflist.GetDefaults()) > 0
	}
	return false
}

func DefaultValueRetrieve(schema *sdcpb.SchemaElem, path *sdcpb.Path, prio int32, intent string) (*types.Update, error) {
	var tv *sdcpb.TypedValue
	var err error
	switch schem := schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Field:
		defaultVal := schem.Field.GetDefault()
		if defaultVal == "" {
			return nil, fmt.Errorf("no defaults defined for schema path: %s", path.ToXPath(false))
		}
		tv, err = utils.Convert(defaultVal, schem.Field.GetType())
		if err != nil {
			return nil, err
		}
	case *sdcpb.SchemaElem_Leaflist:
		listDefaults := schem.Leaflist.GetDefaults()
		if len(listDefaults) == 0 {
			return nil, fmt.Errorf("no defaults defined for schema path: %s", path.ToXPath(false))
		}
		tvlist := make([]*sdcpb.TypedValue, 0, len(listDefaults))
		for _, dv := range schem.Leaflist.GetDefaults() {
			tvelem, err := utils.Convert(dv, schem.Leaflist.GetType())
			if err != nil {
				return nil, fmt.Errorf("error converting default to typed value for %s, type: %s ; value: %s; err: %v", path.ToXPath(false), schem.Leaflist.GetType().GetTypeName(), dv, err)
			}
			tvlist = append(tvlist, tvelem)
		}
		tv = &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{
					Element: tvlist,
				},
			},
		}
	default:
		return nil, fmt.Errorf("no defaults defined for schema path: %s", path.ToXPath(false))
	}

	return types.NewUpdate(path, tv, prio, intent, 0), nil
}
