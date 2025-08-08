package utils

import "strings"

// MapToStrings converts a map into a []string using a formatter function.
func MapToStrings[K comparable, V any](m map[K]V, format func(K, V) string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, format(k, v))
	}
	return out
}

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
