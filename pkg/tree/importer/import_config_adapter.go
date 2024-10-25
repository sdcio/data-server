package importer

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type ImportConfigAdapter interface {
	GetElements() []ImportConfigAdapter
	// GetElement returns the value TreeImportable for a given field.
	// if the field is not present, nil is returned
	GetElement(key string) ImportConfigAdapter
	GetValue() string
	GetTVValue(slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error)
	GetName() string
}
