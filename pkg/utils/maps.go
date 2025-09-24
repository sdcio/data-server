package utils

import "strings"

// MapToString converts a map into a string using a formatter function.
func MapToString[K comparable, V any](m map[K]V, sep string, format func(K, V) string) string {
	var b strings.Builder
	first := true
	for k, v := range m {
		if !first {
			b.WriteString(sep)
		}
		first = false
		b.WriteString(format(k, v))
	}
	return b.String()
}

// MapToStringEmptyMessage converts a map into a string using a formatter function, returning the emptyMessage if the map does not contain entries.
func MapToStringEmptyMessage[K comparable, V any](m map[K]V, sep string, format func(K, V) string, emptyMessage string) string {
	if len(m) == 0 {
		return emptyMessage
	}
	return MapToString(m, sep, format)
}

// MapToSlice executes a function on all the elements of a map, taking in key and value and returns the
// collected results in a slice
func MapToSlice[K comparable, V any, X any](m map[K]V, f func(K, V) X) []X {
	result := make([]X, 0, len(m))
	for k, v := range m {
		result = append(result, f(k, v))
	}
	return result
}

func MapApplyFuncToMap[K comparable, V any, X any](m map[K]V, f func(K, V) X) map[K]X {
	result := map[K]X{}

	for k, v := range m {
		result[k] = f(k, v)
	}
	return result
}
