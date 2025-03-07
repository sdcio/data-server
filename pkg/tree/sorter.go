package tree

func getListEntrySortFunc(parent Entry) func(a, b Entry) int {
	// return the comparison function
	return func(a, b Entry) int {
		keys := parent.GetSchemaKeys()
		var cmpResult int
		for _, v := range keys {
			aLvSlice := a.getChildren()[v].GetHighestPrecedence(LeafVariantSlice{}, false)
			bLvSlice := b.getChildren()[v].GetHighestPrecedence(LeafVariantSlice{}, false)

			aEntry := aLvSlice[0]
			bEntry := bLvSlice[0]

			cmpResult = aEntry.Value().Cmp(bEntry.Value())
			if cmpResult != 0 {
				return cmpResult
			}
		}
		return 0
	}
}
