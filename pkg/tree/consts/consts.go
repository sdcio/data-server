package consts

import "math"

const (
	KeysIndexSep       = "_"
	DefaultValuesPrio  = int32(math.MaxInt32 - 90)
	DefaultsIntentName = "default"
	RunningValuesPrio  = int32(math.MaxInt32 - 100)
	RunningIntentName  = "running"
	ReplaceValuesPrio  = int32(math.MaxInt32 - 110)
	ReplaceIntentName  = "replace"
)
