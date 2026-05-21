package types

import (
	"context"
	"fmt"
	"slices"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type Update struct {
	value            *sdcpb.TypedValue
	priority         int32
	intentName       string
	timestamp        int64
	parent           UpdateParent
	matchedUnionType *sdcpb.SchemaLeafType
}

func NewUpdateFromSdcpbUpdate(parent UpdateParent, u *sdcpb.Update, prio int32, intent string, ts int64) *Update {
	return NewUpdate(parent, u.GetValue(), prio, intent, ts)
}

func NewUpdate(parent UpdateParent, val *sdcpb.TypedValue, prio int32, intent string, ts int64) *Update {
	return &Update{
		value:      val,
		priority:   prio,
		intentName: intent,
		timestamp:  ts,
		parent:     parent,
	}
}

func (u *Update) ToSdcpbUpdate() *sdcpb.Update {
	return &sdcpb.Update{
		Path:  u.parent.SdcpbPath(),
		Value: u.value,
	}
}

func (u *Update) DeepCopy() *Update {

	clonedVal := proto.Clone(u.Value()).(*sdcpb.TypedValue)

	return &Update{
		value:            clonedVal,
		priority:         u.Priority(),
		intentName:       u.intentName,
		timestamp:        u.timestamp,
		parent:           u.parent,
		matchedUnionType: u.matchedUnionType,
	}
}

func (u *Update) Owner() string {
	return u.intentName
}

func (u *Update) SetOwner(owner string) *Update {
	u.intentName = owner
	return u
}

func (u *Update) SetParent(up UpdateParent) {
	if u == nil || up == nil {
		return
	}
	u.parent = up
}

func (u *Update) Priority() int32 {
	return u.priority
}

func (u *Update) SetPriority(prio int32) *Update {
	u.priority = prio
	return u
}

func (u *Update) Timestamp() int64 {
	return u.timestamp
}

func (u *Update) Value() *sdcpb.TypedValue {
	return u.value
}

// EffectiveLeafType returns the matched union branch type if set,
// otherwise returns the fallback (the outer schema type). Used by
// all validators to enforce branch-specific constraints.
func (u *Update) EffectiveLeafType(fallback *sdcpb.SchemaLeafType) *sdcpb.SchemaLeafType {
	if u.matchedUnionType != nil {
		return u.matchedUnionType
	}
	return fallback
}

// WithMatchedType stores the matched union branch type on the Update and
// returns the receiver for fluent chaining.
func (u *Update) WithMatchedType(t *sdcpb.SchemaLeafType) *Update {
	u.matchedUnionType = t
	return u
}

func (u *Update) ValueAsBytes() ([]byte, error) {
	return proto.Marshal(u.value)
}

func (u *Update) SdcpbPath() *sdcpb.Path {
	if u.parent == nil {
		return nil
	}
	return u.parent.SdcpbPath()
}

func (u *Update) String() string {
	path := "<nil>"
	if u.parent != nil {
		path = u.parent.SdcpbPath().ToXPath(false)
	}
	return fmt.Sprintf("path: %s, owner: %s, priority: %d, value: %s", path, u.intentName, u.priority, u.value.String())
}

// EqualSkipPath checks the equality of two updates.
// It however skips comparing paths and timestamps.
// This is a shortcut for performace, for cases in which it is already clear that the path is definately equal.
func (u *Update) Equal(other *Update) bool {
	if u.intentName != other.intentName || u.priority != other.priority {
		return false
	}

	uVal, _ := u.ValueAsBytes()
	oVal, _ := other.ValueAsBytes()
	return slices.Equal(uVal, oVal)
}

// ExpandAndConvertIntent takes a slice of Updates ([]*sdcpb.Update) and converts it into a tree.UpdateSlice, that contains *treetypes.Updates.
func ExpandAndConvertIntent(ctx context.Context, scb utils.SchemaClientBound, intentName string, priority int32, upds []*sdcpb.Update, ts int64) ([]*PathAndUpdate, error) {
	converter := utils.NewConverter(scb)

	// Expands the value, in case of json to single typed value updates
	expandedReqUpdates, err := converter.ExpandUpdates(ctx, upds)
	if err != nil {
		return nil, err
	}

	// temp storage for types.Update of the req. They are to be added later.
	newCacheUpdates := make([]*PathAndUpdate, 0, len(expandedReqUpdates))

	for _, e := range expandedReqUpdates {
		upd := NewUpdate(nil, e.Update.GetValue(), priority, intentName, ts)
		if e.MatchedUnionType != nil {
			upd.WithMatchedType(e.MatchedUnionType)
		}
		newCacheUpdates = append(newCacheUpdates, NewPathAndUpdate(e.Update.GetPath(), upd))
	}
	return newCacheUpdates, nil
}

type UpdateParent interface {
	SdcpbPath() *sdcpb.Path
}
