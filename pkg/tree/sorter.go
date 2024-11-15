package tree

import "cmp"

func getListEntrySortFunc(parent Entry) func(a, b Entry) int {
	return func(a, b Entry) int {
		aIter := a
		bIter := b
		compResult := make([]int, len(parent.GetSchemaKeys()), len(parent.GetSchemaKeys()))
		for i := len(parent.GetSchemaKeys()) - 1; i >= 0; i-- {
			compResult[i] = cmp.Compare(aIter.PathName(), bIter.PathName())
			aIter = aIter.GetParent()
			bIter = bIter.GetParent()
		}
		for _, x := range compResult {
			if x != 0 {
				return x
			}
		}
		return 0
	}
}
