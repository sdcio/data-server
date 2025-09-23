package types

import (
	"strings"
)

// PathSlice is a single Path represented as a string slice
type PathSlice []string

func (p PathSlice) String() string {
	return strings.Join(p, "/")
}

func (p PathSlice) DeepCopy() PathSlice {
	result := make(PathSlice, 0, len(p))
	return append(result, p...)
}

// PathSlices is the slice collection of multiple PathSlice objects.
type PathSlices []PathSlice

func (p PathSlices) StringSlice() []string {
	result := make([]string, 0, len(p))
	for _, x := range p {
		result = append(result, strings.Join(x, "/"))
	}
	return result
}

func (p PathSlices) ToStringSlice() [][]string {
	result := make([][]string, 0, len(p))
	for _, x := range p {
		result = append(result, x)
	}
	return result
}
