package validation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	sdcpbutils "github.com/sdcio/sdc-protos/utils"
)

var (
	ErrValidationError = errors.New("validation error")
)

func Validate(ctx context.Context, e api.Entry, vCfg *config.Validation, taskpoolFactory pool.VirtualPoolFactory) (types.ValidationResults, *types.ValidationStats) {
	// perform validation
	// we use a channel and cumulate all the errors
	validationResultEntryChan := make(chan *types.ValidationResultEntry, 10)
	validationStats := types.NewValidationStats()

	// create a ValidationResult struct
	validationResult := types.ValidationResults{}

	syncWait := &sync.WaitGroup{}
	syncWait.Add(1)
	go func() {
		// read from the validationResult channel
		for e := range validationResultEntryChan {
			validationResult.AddEntry(e)
		}
		syncWait.Done()
	}()

	validationProcessor := NewValidateProcessor(NewValidateProcessorConfig(validationResultEntryChan, validationStats, vCfg))
	validationProcessor.Run(taskpoolFactory, e)
	close(validationResultEntryChan)

	syncWait.Wait()
	return validationResult, validationStats
}

// Validate is the highlevel function to perform validation.
// it will multiplex all the different Validations that need to happen
func validateLevel(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, vCfg *config.Validation) {
	// validate the mandatory statement on this entry
	if e.RemainsToExist() {
		// TODO: Validate Enums
		if !vCfg.DisabledValidators.Mandatory {
			validateMandatory(ctx, e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.Leafref {
			validateLeafRefs(ctx, e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.LeafrefMinMaxAttributes {
			validateLeafListMinMaxAttributes(e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.Pattern {
			validatePattern(e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.MustStatement {
			validateMustStatements(ctx, e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.Length {
			validateLength(e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.Range {
			validateRange(e, resultChan, stats)
		}
		if !vCfg.DisabledValidators.MaxElements {
			validateMinMaxElements(e, resultChan, stats)
		}
	}
}

// validateRange int and uint types (Leaf and Leaflist) define ranges which configured values must lay in.
// validateRange does check this condition.
func validateRange(e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {

	// if no schema present or Field and LeafList Types do not contain any ranges, return there is nothing to check
	if e.GetSchema() == nil || (len(e.GetSchema().GetField().GetType().GetRange()) == 0 && len(e.GetSchema().GetLeaflist().GetType().GetRange()) == 0) {
		return
	}

	lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
	if lv == nil {
		return
	}

	tv := lv.Update.Value()

	var tvs []*sdcpb.TypedValue
	var typeSchema *sdcpb.SchemaLeafType
	// ranges are defined on Leafs or LeafLists.
	switch {
	case len(e.GetSchema().GetField().GetType().GetRange()) != 0:
		// if it is a leaf, extract the value add it as a single value to the tvs slice and check it further down
		tvs = []*sdcpb.TypedValue{tv}
		// we also need the Field/Leaf Type schema
		typeSchema = e.GetSchema().GetField().GetType()
	case len(e.GetSchema().GetLeaflist().GetType().GetRange()) != 0:
		// if it is a leaflist, extract the values them to the tvs slice and check them further down
		tvs = tv.GetLeaflistVal().GetElement()
		// we also need the Field/Leaf Type schema
		typeSchema = e.GetSchema().GetLeaflist().GetType()
	default:
		// if no ranges exist return
		return
	}

	// range through the tvs and check that they are in range
	for _, tv := range tvs {
		// we need to distinguish between unsigned and singned ints
		switch typeSchema.TypeName {
		case "uint8", "uint16", "uint32", "uint64":
			// procede with the unsigned ints
			urnges := sdcpbutils.NewRnges[uint64]()
			// add the defined ranges to the ranges struct
			for _, r := range typeSchema.GetRange() {
				urnges.AddRange(r.Min.Value, r.Max.Value)
			}
			stats.Add(types.StatTypeRange, uint32(len(typeSchema.GetRange())))

			// check the value lays within any of the ranges
			if !urnges.IsWithinAnyRange(tv.GetUintVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", e.SdcpbPath().ToXPath(false), tv.GetUintVal(), urnges.String()), types.ValidationResultEntryTypeError)
			}

		case "int8", "int16", "int32", "int64":
			// procede with the signed ints
			srnges := sdcpbutils.NewRnges[int64]()
			for _, r := range typeSchema.GetRange() {
				// get the value
				min := int64(r.GetMin().GetValue())
				max := int64(r.GetMax().GetValue())
				// take care of the minus sign
				if r.Min.Negative {
					min = min * -1
				}
				if r.Max.Negative {
					max = max * -1
				}
				// add the defined ranges to the ranges struct
				srnges.AddRange(min, max)
			}
			stats.Add(types.StatTypeRange, uint32(len(typeSchema.GetRange())))
			// check the value lays within any of the ranges
			if !srnges.IsWithinAnyRange(tv.GetIntVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", e.SdcpbPath().ToXPath(false), tv.GetIntVal(), srnges.String()), types.ValidationResultEntryTypeError)
			}
		}
	}
}

func validateMinMaxElements(e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	var contSchema *sdcpb.ContainerSchema
	if contSchema = e.GetSchema().GetContainer(); contSchema == nil {
		// if it is not a container, return
		return
	}
	if len(contSchema.GetKeys()) == 0 {
		// if it is not a list, return
		return
	}

	// get all the childs, skipping the key levels
	childs, err := e.GetListChilds()
	if err != nil {
		resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error getting childs for min/max-elements check %v", err), types.ValidationResultEntryTypeError)
	}

	intMin := int(contSchema.GetMinElements())
	intMax := int(contSchema.GetMaxElements())

	// early exit if no specific min/max defined
	if intMin <= 0 && intMax <= 0 {
		return
	}

	// define function to figure out associated owners / intents

	ownersSet := map[string]struct{}{}
	for _, child := range childs {
		childAttributes := child.GetChilds(types.DescendMethodActiveChilds)
		keyName := contSchema.GetKeys()[0].GetName()
		if keyAttr, ok := childAttributes[keyName]; ok {
			highestPrec := keyAttr.GetHighestPrecedence(nil, false, false, false)
			if len(highestPrec) > 0 {
				owner := highestPrec[0].Update.Owner()
				ownersSet[owner] = struct{}{}
			}
		}
	}
	// dedup the owners
	owners := []string{}
	for k := range ownersSet {
		owners = append(owners, k)
	}

	if len(childs) < intMin {
		for _, owner := range owners {
			resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("Min-Elements violation on %s expected %d actual %d", e.SdcpbPath().ToXPath(false), intMin, len(childs)), types.ValidationResultEntryTypeError)
		}
	}
	if intMax > 0 && len(childs) > intMax {
		for _, owner := range owners {
			resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("Max-Elements violation on %s expected %d actual %d", e.SdcpbPath().ToXPath(false), intMax, len(childs)), types.ValidationResultEntryTypeError)
		}
	}
	stats.Add(types.StatTypeMinMaxElementsList, 1)
}

// validateLeafListMinMaxAttributes validates the Min-, and Max-Elements attribute of the Entry if it is a Leaflists.
func validateLeafListMinMaxAttributes(e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := e.GetSchema().GetLeaflist(); schema != nil {
		if schema.GetMinElements() > 0 || schema.GetMaxElements() < math.MaxUint64 {
			if lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false); lv != nil {
				tv := lv.Update.Value()

				if val := tv.GetLeaflistVal(); val != nil {
					// check minelements if set
					if schema.GetMinElements() > 0 && len(val.GetElement()) < int(schema.GetMinElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d min-elements but only %d elements are present", e.SdcpbPath().ToXPath(false), schema.MinElements, len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
					// check maxelements if set
					if uint64(len(val.GetElement())) > uint64(schema.GetMaxElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d max-elements but %d elements are present", e.SdcpbPath().ToXPath(false), schema.GetMaxElements(), len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
				}
				stats.Add(types.StatTypeMinMaxElementsLeaflist, 1)
			}
		}
	}
}

func validateLength(e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := e.GetSchema().GetField(); schema != nil {

		if len(schema.GetType().GetLength()) == 0 {
			return
		}

		lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		actualLength := utf8.RuneCountInString(value)

		for _, lengthDef := range schema.GetType().GetLength() {

			if lengthDef.Min.Value <= uint64(actualLength) && uint64(actualLength) <= lengthDef.Max.Value {
				// continue if the length is within the range
				continue
			}
			// this is already the failure case
			lenghts := []string{}
			for _, lengthDef := range schema.GetType().GetLength() {
				lenghts = append(lenghts, fmt.Sprintf("%d..%d", lengthDef.Min.Value, lengthDef.Max.Value))
			}
			resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("error length of Path: %s, Value: %s not within allowed length %s", e.SdcpbPath().ToXPath(false), value, strings.Join(lenghts, ", ")), types.ValidationResultEntryTypeError)
		}
		stats.Add(types.StatTypeLength, uint32(len(schema.GetType().GetLength())))
	}
}

func validatePattern(e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := e.GetSchema().GetField(); schema != nil {
		if len(schema.GetType().GetPatterns()) == 0 {
			return
		}

		lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		for _, pattern := range schema.GetType().GetPatterns() {
			if p := pattern.GetPattern(); p != "" {
				matched, err := regexp.MatchString(p, value)
				if err != nil {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("failed compiling regex %s defined for %s", p, e.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
					continue
				}
				if (!matched && !pattern.Inverted) || (pattern.GetInverted() && matched) {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("value %s of %s does not match regex %s (inverted: %t)", value, e.SdcpbPath().ToXPath(false), p, pattern.GetInverted()), types.ValidationResultEntryTypeError)
				}
			}

		}
		stats.Add(types.StatTypePattern, uint32(len(schema.GetType().GetPatterns())))
	}
}

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
					log.Error(ErrValidationError, "mandatory attribute could not be found as child, field or choice", "path", e.SdcpbPath().ToXPath(false), "attribute", c.Name)
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
