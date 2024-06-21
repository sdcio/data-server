package tree

import (
	"strings"
)

type PathSlice [][]string

func (p PathSlice) StringSlice() []string {
	result := make([]string, 0, len(p))
	for _, x := range p {
		result = append(result, strings.Join(x, "/"))
	}
	return result
}
