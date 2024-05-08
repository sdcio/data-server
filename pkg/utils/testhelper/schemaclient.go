package testhelper

import (
	"context"
	"testing"

	"github.com/sdcio/data-server/mocks/mockschema"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

func ConfigureSchemaClientMock(_ *testing.T, scm *mockschema.MockClient) {

	// configure .ToPath()
	pathMap := GetToPathMap()
	scm.EXPECT().ToPath(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, in *sdcpb.ToPathRequest, opts ...grpc.CallOption) (*sdcpb.ToPathResponse, error) {

			return &sdcpb.ToPathResponse{
				Path: pathMap[PathMapIndex(in.GetPathElement())],
			}, nil
		},
	)

	// configure .GetSchema()
	schemaMap := GetSchemaMap()
	scm.EXPECT().GetSchema(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, in *sdcpb.GetSchemaRequest, _ ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
			idx := PathMapIndex(utils.ToStrings(in.GetPath(), false, true))
			return &sdcpb.GetSchemaResponse{
				Schema: schemaMap[idx],
			}, nil
		},
	)
}

// GetToPathMap returns a lookup map for a given set of test paths that allows conversion from string based paths to sdcpb.Path
func GetToPathMap() map[string]*sdcpb.Path {

	// index for the ToPath() function
	pathMap := map[string]*sdcpb.Path{}
	pathMap[""] = &sdcpb.Path{}

	pathMap[PathMapIndex([]string{"interface"})] = createPath(pathMap[""], "interface", nil)

	// interface ethernet0/0
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0"})] = createPath(pathMap[""], "interface", map[string]string{"name": "ethernet-0/0"})

	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "name"})] = createPath(pathMap["interface/ethernet-0/0"], "name", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "description"})] = createPath(pathMap["interface/ethernet-0/0"], "description", nil)

	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface"})] = createPath(pathMap["interface/ethernet-0/0"], "subinterface", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "0"})] = createPath(pathMap["interface/ethernet-0/0/subinterface"], "subinterface", map[string]string{"index": "0"})
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "0", "index"})] = createPath(pathMap["interface/ethernet-0/0/subinterface/0"], "index", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "0", "description"})] = createPath(pathMap["interface/ethernet-0/0/subinterface/0"], "description", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "1"})] = createPath(pathMap["interface/ethernet-0/0/subinterface"], "subinterface", map[string]string{"index": "1"})
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"})] = createPath(pathMap["interface/ethernet-0/0/subinterface/1"], "index", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"})] = createPath(pathMap["interface/ethernet-0/0/subinterface/1"], "description", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "2"})] = createPath(pathMap["interface/ethernet-0/0/subinterface"], "subinterface", map[string]string{"index": "2"})
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "2", "index"})] = createPath(pathMap["interface/ethernet-0/0/subinterface/2"], "index", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/0", "subinterface", "2", "description"})] = createPath(pathMap["interface/ethernet-0/0/subinterface/2"], "description", nil)

	// interface ethernet0/1
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1"})] = createPath(pathMap[""], "interface", map[string]string{"name": "ethernet-0/1"})

	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "name"})] = createPath(pathMap["interface/ethernet-0/1"], "name", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "description"})] = createPath(pathMap["interface/ethernet-0/1"], "description", nil)

	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface"})] = createPath(pathMap["interface/ethernet-0/1"], "subinterface", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "0"})] = createPath(pathMap["interface/ethernet-0/1/subinterface"], "subinterface", map[string]string{"index": "0"})
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "0", "description"})] = createPath(pathMap["interface/ethernet-0/1/subinterface/0"], "description", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "0", "index"})] = createPath(pathMap["interface/ethernet-0/1/subinterface/0"], "index", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "1"})] = createPath(pathMap["interface/ethernet-0/1/subinterface"], "subinterface", map[string]string{"index": "1"})
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "1", "description"})] = createPath(pathMap["interface/ethernet-0/1/subinterface/1"], "description", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "1", "index"})] = createPath(pathMap["interface/ethernet-0/1/subinterface/1"], "index", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "2"})] = createPath(pathMap["interface/ethernet-0/1/subinterface"], "subinterface", map[string]string{"index": "2"})
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "2", "description"})] = createPath(pathMap["interface/ethernet-0/1/subinterface/2"], "description", nil)
	pathMap[PathMapIndex([]string{"interface", "ethernet-0/1", "subinterface", "2", "index"})] = createPath(pathMap["interface/ethernet-0/1/subinterface/2"], "index", nil)

	return pathMap
}

func GetSchemaMap() map[string]*sdcpb.SchemaElem {

	// index for the ToPath() function
	responseMap := map[string]*sdcpb.SchemaElem{}

	// root
	responseMap[""] = createSchemaContainer("__root__", nil, []string{"interface", "network-instance"}, nil)

	// !!! All subsequent Maps do carry the "" (empty string) as the first elem, because that represents the root elem

	// interface
	responseMap[PathMapIndex([]string{"interface", "name"})] = createSchemaField("name", "string")
	responseMap[PathMapIndex([]string{"interface", "description"})] = createSchemaField("description", "string")
	responseMap[PathMapIndex([]string{"interface"})] = createSchemaContainer("interface", []string{"name"}, []string{"subinterface"}, []*sdcpb.LeafSchema{
		responseMap[PathMapIndex([]string{"interface", "name"})].GetField(),
		responseMap[PathMapIndex([]string{"interface", "description"})].GetField(),
	},
	)

	// interface / subinterface
	responseMap[PathMapIndex([]string{"interface", "subinterface", "index"})] = createSchemaField("index", "uint8")
	responseMap[PathMapIndex([]string{"interface", "subinterface", "description"})] = createSchemaField("description", "string")
	responseMap[PathMapIndex([]string{"interface", "subinterface"})] = createSchemaContainer("subinterface", []string{"index"}, []string{"index", "description"}, []*sdcpb.LeafSchema{
		responseMap[PathMapIndex([]string{"interface", "subinterface", "index"})].GetField(),
		responseMap[PathMapIndex([]string{"interface", "subinterface", "description"})].GetField(),
	})

	// network-instance
	responseMap[PathMapIndex([]string{"network-instance", "name"})] = createSchemaField("name", "string")
	responseMap[PathMapIndex([]string{"network-instance", "description"})] = createSchemaField("description", "string")
	responseMap[PathMapIndex([]string{"network-instance"})] = createSchemaContainer("network-instance", []string{"name"}, []string{"name", "description"}, []*sdcpb.LeafSchema{
		responseMap[PathMapIndex([]string{"network-instance", "name"})].GetField(),
		responseMap[PathMapIndex([]string{"network-instance", "description"})].GetField(),
	})

	return responseMap
}

// createPath takes an existing path, copies its elements, appends the new elem with the given name and the provided keys
func createPath(p *sdcpb.Path, elemName string, keys map[string]string) *sdcpb.Path {
	result := utils.CopyPath(p)

	result.Elem = append(result.Elem, &sdcpb.PathElem{
		Name: elemName,
		Key:  keys,
	})
	return result
}

// createSchemaContainer generate a container schema elem
func createSchemaContainer(name string, keys []string, children []string, fields []*sdcpb.LeafSchema) *sdcpb.SchemaElem {

	// process the keys
	sKeys := []*sdcpb.LeafSchema{}
	for _, k := range keys {
		sKeys = append(sKeys, &sdcpb.LeafSchema{
			Name: k,
		})
	}

	// build and return the schema element
	return &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{
			Container: &sdcpb.ContainerSchema{
				Name:     name,
				Keys:     sKeys,
				Children: children,
				Fields:   fields,
			},
		},
	}
}

// createSchemaField generate a field schema elem
func createSchemaField(name string, typ string) *sdcpb.SchemaElem {
	return &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{
			Field: &sdcpb.LeafSchema{
				Name: name,
				Type: &sdcpb.SchemaLeafType{
					Type: typ,
				},
			},
		},
	}
}

type TestConfig struct {
	Interf          []*Interf          `json:"interface,omitempty"`
	NetworkInstance []*NetworkInstance `json:"network-instance,omitempty"`
}

type Interf struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Subinterface []*Subinterface `json:"subinterface,omitempty"`
}

type Subinterface struct {
	Index       int    `json:"index"`
	Description string `json:"description,omitempty"`
}

type NetworkInstance struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
