package tree

import "cmp"

func getListEntrySortFunc(parent Entry) func(a, b Entry) int {
	// return the comparison function
	return func(a, b Entry) int {
		// we need to iterate through a and b, so assign them to iter variables
		aIter := a
		bIter := b
		// comparison result store
		compResult := make([]int, len(parent.GetSchemaKeys()), len(parent.GetSchemaKeys()))
		// we iterate backwards through the keys, since we're using the Pathnames of the elements up towards the parent
		for i := len(parent.GetSchemaKeys()) - 1; i >= 0; i-- {
			// store the comparison result of the actual level
			compResult[i] = cmp.Compare(aIter.PathName(), bIter.PathName())
			// iterate up a level (which will be the key levels)
			aIter = aIter.GetParent()
			bIter = bIter.GetParent()
		}
		// iterate through the results in normal order (non-reversed)
		for _, x := range compResult {
			// if the compResult is different from 0, the elements differ and the
			// ordering of the two elements is clear
			if x != 0 {
				return x
			}
		}
		return 0
	}
}
