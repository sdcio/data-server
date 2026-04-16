package validation

import (
	"context"
	"fmt"
	"slices"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// validateMandatory validates that all the mandatory attributes,
// defined by the schema are present either in the tree or in the index.
func validateMandatory(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	log := logger.FromContext(ctx)
	if !e.RemainsToExist() {
		return
	}
	schema := e.GetSchema()
	if schema != nil {
		switch schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			containerSchema := schema.GetContainer()
			for _, c := range containerSchema.GetMandatoryChildrenConfig() {
				attributes := []string{}
				choiceName := ""
				// check if it is a ChildContainer
				if slices.Contains(containerSchema.GetChildren(), c.Name) {
					attributes = append(attributes, c.Name)
				}

				// check if it is a Key
				if slices.ContainsFunc(containerSchema.GetKeys(), func(x *sdcpb.LeafSchema) bool {
					return x.Name == c.Name
				}) {
					attributes = append(attributes, c.Name)
				}

				// check if it is a Field
				if slices.ContainsFunc(containerSchema.GetFields(), func(x *sdcpb.LeafSchema) bool {
					return x.Name == c.Name
				}) {
					attributes = append(attributes, c.Name)
				}

				// otherwise it will probably be a choice
				if len(attributes) == 0 {
					choice := containerSchema.GetChoiceInfo().GetChoiceByName(c.Name)
					if choice != nil {
						attributes = append(attributes, choice.GetAllAttributes()...)
						choiceName = c.Name
					}
				}

				if len(attributes) == 0 {
					log.Error(ErrValidation, "mandatory attribute could not be found as child, field or choice", "path", e.SdcpbPath().ToXPath(false), "attribute", c.Name)
				}

				validateMandatoryWithKeys(ctx, e, len(containerSchema.GetKeys()), attributes, choiceName, resultChan)
			}
			stats.Add(types.StatTypeMandatory, uint32(len(containerSchema.GetMandatoryChildrenConfig())))
		}
	}
}

// validateMandatoryWithKeys steps down the tree, passing the key levels and checking the existence of the mandatory.
// attributes is a string slice, it will be checked that at least of the the given attributes is defined
// !Not checking all of these are defined (call multiple times with single entry in attributes for that matter)!
func validateMandatoryWithKeys(ctx context.Context, e api.Entry, level int, attributes []string, choiceName string, resultChan chan<- *types.ValidationResultEntry) {
	if e.ShouldDelete() {
		return
	}
	// need to step down the tree until we're beyond the key levels to check the mandatory attributes, if level is > 0, we are still in the key levels
	if level > 0 {
		for _, c := range e.GetChilds(types.DescendMethodActiveChilds) {
			validateMandatoryWithKeys(ctx, c, level-1, attributes, choiceName, resultChan)
		}
		return
	}

	success := false
	existsInTree := false
	var v api.Entry
	// iterate over the attributes make sure any of these exists
	for _, attr := range attributes {
		// first check if the mandatory value is set via the intent, e.g. part of the tree already
		v, existsInTree = e.GetChilds(types.DescendMethodActiveChilds)[attr]
		// if exists and remains to Exist
		if existsInTree && v.RemainsToExist() {
			// set success to true and break the loop
			success = true
			break
		}
	}
	// if not the path exists in the tree and is not to be deleted, then lookup in the paths index of the store
	// and see if such path exists, if not raise the error
	if !success {
		// if it is not a choice
		if choiceName == "" {
			resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory child %s does not exist, path: %s", attributes, e.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
			return
		}
		// if it is a mandatory choice
		resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory choice %s [attributes: %s] does not exist, path: %s", choiceName, attributes, e.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
		return
	}
}
