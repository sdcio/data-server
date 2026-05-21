package importer

import (
	"context"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ImportConfigAdapter is used by the ImportConfig() of the Tree. It allows to import hierarchically organized config data into the tree with little overhead.
// implementation for JSON and XML do exist.
type ImportConfigAdapter interface {
	ImportConfigAdapterElement
	GetDeletes() *sdcpb.PathSet
	GetName() string
	GetPriority() int32
	GetNonRevertive() bool
}

type ImportConfigAdapterElement interface {
	// GetElements returns the elements of a certain level.
	// This can be maps or arrays.
	// In case of arrays, the name is the key of the map that pointed to the List and
	// the content are the array values one after the other.
	// For maps, the key is the GetName() result and the GetElements() result is the referenced value
	GetElements() []ImportConfigAdapterElement
	// GetElement returns the value TreeImportable for a given field.
	// if the field is not present, nil is returned
	GetElement(key string) ImportConfigAdapterElement
	// GetKeyValue can be called on Leafs or LeafList elements to retrieve the underlaying value
	// When and were to expect a Leafs or LeafList is defined by the yang schema.
	// The String value is typically used for the keys.
	GetKeyValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (string, error)
	// GetTVValue returns the TypedValue based value defined via the SchemaLeafType. Can also only be called on Leafs or LeafLists.
	// For union-typed leaves the second return value is the matched branch SchemaLeafType; for non-union leaves it is nil. Proto/XML import set it via InferUnionMemberFromTypedValue (RFC 7950 §9.12 first match).
	GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, *sdcpb.SchemaLeafType, error)
	// returns the name of the actual Level.
	GetName() string
}
