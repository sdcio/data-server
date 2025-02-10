package utils

import (
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/cache"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func DefaultValueExists(schema *sdcpb.SchemaElem) bool {
	switch schem := schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Field:
		return schem.Field.GetDefault() != ""
	case *sdcpb.SchemaElem_Leaflist:
		return schem.Leaflist.GetDefaults() != nil
	}
	return false
}

func DefaultValueRetrieve(schema *sdcpb.SchemaElem, path []string, prio int32, intent string) (*cache.Update, error) {
	var tv *sdcpb.TypedValue
	var err error
	switch schem := schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Field:
		defaultVal := schem.Field.GetDefault()
		if defaultVal == "" {
			return nil, fmt.Errorf("no defaults defined for schema path: %s", strings.Join(path, "->"))
		}
		tv, err = Convert(defaultVal, schem.Field.GetType())
		if err != nil {
			return nil, err
		}
	case *sdcpb.SchemaElem_Leaflist:
		listDefaults := schem.Leaflist.GetDefaults()
		if len(listDefaults) == 0 {
			return nil, fmt.Errorf("no defaults defined for schema path: %s", strings.Join(path, "->"))
		}
		tvlist := make([]*sdcpb.TypedValue, 0, len(listDefaults))
		for _, dv := range schem.Leaflist.GetDefaults() {
			tvelem, err := Convert(dv, schem.Leaflist.GetType())
			if err != nil {
				return nil, fmt.Errorf("error converting default to typed value for %s, type: %s ; value: %s; err: %v", strings.Join(path, " -> "), schem.Leaflist.GetType().GetTypeName(), dv, err)
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
		return nil, fmt.Errorf("no defaults defined for schema path: %s", strings.Join(path, "->"))
	}

	// convert value to []byte for cache insertion
	val, err := proto.Marshal(tv)
	if err != nil {
		return nil, err
	}

	return cache.NewUpdate(path, val, prio, intent, 0), nil
}
