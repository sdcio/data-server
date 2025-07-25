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
	value      *sdcpb.TypedValue
	priority   int32
	intentName string
	timestamp  int64
	path       *sdcpb.Path
}

func NewUpdateFromSdcpbUpdate(u *sdcpb.Update, prio int32, intent string, ts int64) *Update {
	return NewUpdate(u.GetPath(), u.GetValue(), prio, intent, ts)
}

func NewUpdate(path *sdcpb.Path, val *sdcpb.TypedValue, prio int32, intent string, ts int64) *Update {
	return &Update{
		value:      val,
		priority:   prio,
		intentName: intent,
		timestamp:  ts,
		path:       path,
	}
}

func (u *Update) DeepCopy() *Update {

	clonedVal := proto.Clone(u.Value()).(*sdcpb.TypedValue)

	return &Update{
		value:      clonedVal,
		priority:   u.Priority(),
		intentName: u.intentName,
		timestamp:  u.timestamp,
		path:       u.path.DeepCopy(),
	}
}

func (u *Update) Owner() string {
	return u.intentName
}

func (u *Update) SetOwner(owner string) {
	u.intentName = owner
}

func (u *Update) Priority() int32 {
	return u.priority
}

func (u *Update) SetPriority(prio int32) {
	u.priority = prio
}

func (u *Update) Timestamp() int64 {
	return u.timestamp
}

func (u *Update) Value() *sdcpb.TypedValue {
	return u.value
}

func (u *Update) ValueAsBytes() ([]byte, error) {
	return proto.Marshal(u.value)
}

func (u *Update) String() string {
	return fmt.Sprintf("path: %s, owner: %s, priority: %d, value: %s", u.path, u.intentName, u.priority, u.value.String())
}

func (u *Update) Path() *sdcpb.Path {
	if u.path == nil {
		return &sdcpb.Path{}
	}
	return u.path
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
func ExpandAndConvertIntent(ctx context.Context, scb utils.SchemaClientBound, intentName string, priority int32, upds []*sdcpb.Update, ts int64) (UpdateSlice, error) {
	converter := utils.NewConverter(scb)

	// Expands the value, in case of json to single typed value updates
	expandedReqUpdates, err := converter.ExpandUpdates(ctx, upds)
	if err != nil {
		return nil, err
	}

	// temp storage for types.Update of the req. They are to be added later.
	newCacheUpdates := make(UpdateSlice, 0, len(expandedReqUpdates))

	for _, u := range expandedReqUpdates {
		// construct the types.Update
		newCacheUpdates = append(newCacheUpdates, NewUpdate(u.GetPath(), u.GetValue(), priority, intentName, ts))
	}
	return newCacheUpdates, nil
}
