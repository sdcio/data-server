package tree

import (
	"strings"
)

type PathSlice [][]string

func (p PathSlice) StringSlice() []string {
	result := make([]string, len(p), 0)
	for _, x := range p {
		result = append(result, strings.Join(x, "/"))
	}
	return result
}
