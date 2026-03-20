package consts

import "math"

const (
	KeysIndexSep       = "_"
	DefaultValuesPrio  = int32(math.MaxInt32 - 90)
	DefaultsIntentName = "default"
	RunningValuesPrio  = int32(math.MaxInt32 - 100)
	RunningIntentName  = "running"
	RevertRunningPrio  = int32(math.MaxInt32 - 105)
	RevertRunningName  = "revrun"
	ReplaceValuesPrio  = int32(math.MaxInt32 - 110)
	ReplaceIntentName  = "replace"

	// UserSettableMax maximum priority allowed to be set by a user.
	UserSettableMax = math.MaxInt32 - 500
)
