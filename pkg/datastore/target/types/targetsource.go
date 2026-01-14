package types

import (
	"context"

	"github.com/beevik/etree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TargetSource interface {
	// ToJson returns the Tree contained structure as JSON
	// use e.g. json.MarshalIndent() on the returned struct
	ToJson(onlyNewOrUpdated bool) (any, error)
	// ToJsonIETF returns the Tree contained structure as JSON_IETF
	// use e.g. json.MarshalIndent() on the returned struct
	ToJsonIETF(onlyNewOrUpdated bool) (any, error)
	ToXML(onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (*etree.Document, error)
	ToProtoUpdates(ctx context.Context, onlyNewOrUpdated bool) ([]*sdcpb.Update, error)
	ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error)
}
