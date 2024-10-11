package tree

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (s *sharedEntryAttributes) ToJson(onlyNewOrUpdated bool) (any, error) {
	result, err := s.toJsonInternal(onlyNewOrUpdated, false, "")
	if result == nil {
		return map[string]any{}, err
	}
	return result, err
}

func (s *sharedEntryAttributes) ToJsonIETF(onlyNewOrUpdated bool) (any, error) {
	result, err := s.toJsonInternal(onlyNewOrUpdated, true, "")
	if result == nil {
		return map[string]any{}, err
	}
	return result, err
}

// ToJson returns the Branch of the tree as a struct that can be marshalled as JSON
// If the ietf parameter is set to true, JSON_IETF encoding is used.
// The actualPrefix is used only for the JSON_IETF encoding and can be ignored for JSON
// In the initial / users call with ietf == true, actualPrefix should be set to ""
func (s *sharedEntryAttributes) toJsonInternal(onlyNewOrUpdated bool, ietf bool, actualPrefix string) (any, error) {
	switch s.schema.GetSchema().(type) {
	case nil:
		// we're operating on a key level, no schema attached, but the
		// ancestor is a list with keys.
		result := map[string]any{}

		for k, c := range s.filterActiveChoiceCaseChilds() {
			var prefix string
			key := k
			// if JSON_IETF is requested, acquire prefix
			if ietf {
				// get the prefix
				prefix = utils.GetSchemaElemModuleName(c.GetSchema())
				// only add prefix if it is not empty and is different from the
				// given actualPrefix
				if prefix != "" && prefix != actualPrefix {
					key = fmt.Sprintf("%s:%s", prefix, key)
				}
			}
			// recurse the call
			js, err := c.toJsonInternal(onlyNewOrUpdated, ietf, prefix)
			if err != nil {
				return nil, err
			}
			if js != nil {
				result[key] = js
			}
		}
		if len(result) == 0 {
			return nil, nil
		}
		return result, nil
	case *sdcpb.SchemaElem_Container:
		if len(s.GetSchemaKeys()) > 0 {
			// if the container contains keys, then it is a list
			// hence must be rendered as an array
			childs, err := s.FilterChilds(nil)
			if err != nil {
				return nil, err
			}
			result := make([]any, 0, len(childs))
			for _, c := range childs {
				j, err := c.toJsonInternal(onlyNewOrUpdated, ietf, actualPrefix)
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
		}

		// otherwise this is a map
		result := map[string]any{}
		for k, c := range s.filterActiveChoiceCaseChilds() {
			var prefix string
			key := k
			if ietf {
				// get the prefix
				prefix = utils.GetSchemaElemModuleName(c.GetSchema())
				// only add prefix if it is not empty and is different from the
				// given actualPrefix
				if prefix != "" && prefix != actualPrefix {
					key = fmt.Sprintf("%s:%s", prefix, key)
				}
			}
			js, err := c.toJsonInternal(onlyNewOrUpdated, ietf, prefix)
			if err != nil {
				return nil, err
			}
			if js != nil {
				result[key] = js
			}
		}
		if len(result) == 0 {
			return nil, nil
		}
		return result, nil
	case *sdcpb.SchemaElem_Leaflist, *sdcpb.SchemaElem_Field:
		le := s.leafVariants.GetHighestPrecedence(false)
		if onlyNewOrUpdated && !(le.IsNew || le.IsUpdated) {
			return nil, nil
		}
		v, err := le.Update.Value()
		if err != nil {
			return nil, err
		}
		return utils.GetJsonValue(v), nil
	}
	return nil, fmt.Errorf("unable to convert to json (%s)", s.Path())
}
