package tree

import (
	"strings"
)

type PathsSlice [][]string

func (p PathsSlice) StringSlice() []string {
	result := make([]string, 0, len(p))
	for _, x := range p {
		result = append(result, strings.Join(x, "/"))
	}
	return result
}
