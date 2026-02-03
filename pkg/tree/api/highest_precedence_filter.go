package api

type HighestPrecedenceFilter func(le *LeafEntry) bool

func HighestPrecedenceFilterAll(le *LeafEntry) bool {
	return true
}
func HighestPrecedenceFilterWithoutNew(le *LeafEntry) bool {
	return !le.IsNew
}
func HighestPrecedenceFilterWithoutDeleted(le *LeafEntry) bool {
	return !le.Delete
}
