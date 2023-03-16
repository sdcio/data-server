package conversion

import (
	"fmt"
	"strconv"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// URnges represents a collection of rng (range)
type URnges struct {
	rnges []*URng
}

// URng represents a single unsigned range
type URng struct {
	min uint64
	max uint64
}

func NewUrnges(rangeDefinition string, min, max uint64) *URnges {
	r := &URnges{}
	r.parse(rangeDefinition, min, max)
	return r
}

func (r *URng) isInRange(value uint64) bool {
	// return the result
	return r.min <= value && value <= r.max
}

func (r *URng) String() string {
	// return the result
	return fmt.Sprintf("%d..%d", r.min, r.max)
}

func (r *URnges) isWithinAnyRange(value string) (*schemapb.TypedValue, error) {
	uintValue, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, err
	}
	// create the TypedValue already
	tv := &schemapb.TypedValue{
		Value: &schemapb.TypedValue_UintVal{
			UintVal: uintValue,
		},
	}

	// if no ranges defined, return the tv
	if len(r.rnges) == 0 {
		return tv, nil
	}
	// check the ranges
	for _, rng := range r.rnges {
		if rng.isInRange(uintValue) {
			return tv, nil
		}
	}
	return nil, fmt.Errorf("%q not within ranges", value)
}

func (r *URnges) parse(rangeDef string, min, max uint64) error {

	// to make sure the value is in the general limits of the datatype uint8|16|32|64
	// we add the min max as a seperate additional range
	r.rnges = append(r.rnges, &URng{
		min: min,
		max: max,
	})

	// process all the schema based range definitions
	rangeStrings := strings.Split(rangeDef, "|")
	for _, rangeString := range rangeStrings {
		range_minmax := strings.Split(rangeString, "..")

		switch len(range_minmax) {
		case 1: // we do not have a real range but an exact number e.g. "45"
			exactValue, err := strconv.ParseUint(range_minmax[0], 10, 64)
			if err != nil {
				return err
			}
			r.rnges = append(r.rnges, &URng{
				min: exactValue,
				max: exactValue,
			})
		case 2: // we do have a real range e.g. "8..25"
			var err error
			if range_minmax[0] != "min" {
				min, err = strconv.ParseUint(range_minmax[0], 10, 64)
				if err != nil {
					return err
				}
			}
			if range_minmax[1] != "max" {
				max, err = strconv.ParseUint(range_minmax[1], 10, 64)
				if err != nil {
					return err
				}
			}
			r.rnges = append(r.rnges, &URng{
				min: min,
				max: max,
			})
		default: // any other case is illegal
			return fmt.Errorf("illegal range expression %q", rangeString)
		}
	}
	return nil
}
