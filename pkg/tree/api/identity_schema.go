package api

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// NamespaceURI returns the XML namespace URI for the schema element.
func NamespaceURI(schema *sdcpb.SchemaElem) string {
	if schema == nil {
		return ""
	}
	return utils.GetNamespaceFromGetSchema(schema)
}

// ValidateIdentityMatchesSchema ensures identity.Module matches the schema's
// defining module when both are set.
func ValidateIdentityMatchesSchema(id NodeIdentity, schema *sdcpb.SchemaElem) error {
	if schema == nil || id.Module == "" {
		return nil
	}
	mod := utils.GetSchemaElemModuleName(schema)
	if mod == "" || mod == id.Module {
		return nil
	}
	return fmt.Errorf("node identity module %q does not match schema module %q for local name %q", id.Module, mod, id.Local)
}
