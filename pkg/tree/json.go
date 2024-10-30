package tree

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (s *sharedEntryAttributes) ToJson(onlyNewOrUpdated bool) (any, error) {
	result, err := s.toJsonInternal(onlyNewOrUpdated, false)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, err
}

func (s *sharedEntryAttributes) ToJsonIETF(onlyNewOrUpdated bool) (any, error) {
	result, err := s.toJsonInternal(onlyNewOrUpdated, true)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, err
}

// ToJson returns the Branch of the tree as a struct that can be marshalled as JSON
// If the ietf parameter is set to true, JSON_IETF encoding is used.
// The actualPrefix is used only for the JSON_IETF encoding and can be ignored for JSON
// In the initial / users call with ietf == true, actualPrefix should be set to ""
func (s *sharedEntryAttributes) toJsonInternal(onlyNewOrUpdated bool, ietf bool) (any, error) {
	switch s.schema.GetSchema().(type) {
	case nil:
		// we're operating on a key level, no schema attached, but the
		// ancestor is a list with keys.
		result := map[string]any{}

		for key, c := range s.filterActiveChoiceCaseChilds() {
			ancest, _ := s.GetFirstAncestorWithSchema()
			prefixedKey := jsonGetIetfPrefixConditional(key, c, ancest, ietf)
			// recurse the call
			js, err := c.toJsonInternal(onlyNewOrUpdated, ietf)
			if err != nil {
				return nil, err
			}
			if js != nil {
				result[prefixedKey] = js
			}
		}
		if len(result) == 0 {
			return nil, nil
		}
		jsonAddKeyElements(s, result)
		return result, nil
	case *sdcpb.SchemaElem_Container:
		switch {
		case len(s.GetSchemaKeys()) > 0:
			// if the container contains keys, then it is a list
			// hence must be rendered as an array
			childs, err := s.FilterChilds(nil)
			if err != nil {
				return nil, err
			}
			result := make([]any, 0, len(childs))
			for _, c := range childs {
				j, err := c.toJsonInternal(onlyNewOrUpdated, ietf)
				if err != nil {
					return nil, err
				}
				if j != nil {
					result = append(result, j)
				}
			}
			if len(result) == 0 {
				return nil, nil
			}
			return result, nil
		case len(s.childs) == 0 && s.schema.GetContainer().IsPresence:
			// Presence container without any childs
			return map[string]any{}, nil
		default:
			// otherwise this is a map
			result := map[string]any{}
			for key, c := range s.filterActiveChoiceCaseChilds() {
				prefixedKey := jsonGetIetfPrefixConditional(key, c, s, ietf)
				js, err := c.toJsonInternal(onlyNewOrUpdated, ietf)
				if err != nil {
					return nil, err
				}
				if js != nil {
					result[prefixedKey] = js
				}
			}
			if len(result) == 0 {
				return nil, nil
			}
			return result, nil
		}

	case *sdcpb.SchemaElem_Leaflist, *sdcpb.SchemaElem_Field:
		if s.leafVariants.shouldDelete() {
			return nil, nil
		}
		le := s.leafVariants.GetHighestPrecedence(false)
		if onlyNewOrUpdated && !(le.IsNew || le.IsUpdated) {
			return nil, nil
		}
		v, err := le.Update.Value()
		if err != nil {
			return nil, err
		}
		return utils.GetJsonValue(v, ietf)
	}
	return nil, fmt.Errorf("unable to convert to json (%s)", s.Path())
}

// jsonAddIetfPrefixConditional adds the module name
func jsonGetIetfPrefixConditional(key string, a Entry, b Entry, ietf bool) string {
	// if not ietf, then we do not need module prefixes
	if !ietf {
		return key
	}
	aModule := utils.GetSchemaElemModuleName(a.GetSchema())
	if aModule == utils.GetSchemaElemModuleName(b.GetSchema()) {
		return key
	}
	return fmt.Sprintf("%s:%s", aModule, key)
}

// xmlAddKeyElements determines the keys of a certain Entry in the tree and adds those to the
// element if they do not already exist.
func jsonAddKeyElements(s Entry, dict map[string]any) {
	// retrieve the parent schema, we need to extract the key names
	// values are the tree level names
	parentSchema, levelsUp := s.GetFirstAncestorWithSchema()
	// from the parent we get the keys as slice
	schemaKeys := parentSchema.GetSchemaKeys()
	var treeElem Entry = s
	// the keys do match the levels up in the tree in reverse order
	// hence we init i with levelUp and count down
	for i := levelsUp - 1; i >= 0; i-- {
		// skip if the element already exists
		if _, exists := dict[schemaKeys[i]]; !exists {
			// and finally we create the patheleme key attributes
			dict[schemaKeys[i]] = treeElem.PathName()
			treeElem = treeElem.GetParent()
		}
	}
}
