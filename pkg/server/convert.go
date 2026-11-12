package server

import (
	"context"

	"github.com/sdcio/data-server/pkg/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// intentFromSdcpb converts a transaction intent; a set delete flag wins over
// update content.
func intentFromSdcpb(pb *sdcpb.TransactionIntent) *api.Intent {
	in := &api.Intent{Name: pb.GetIntent()}
	if pb.GetDelete() {
		in.Spec.Delete = &api.Delete{IgnoreNoExist: pb.GetDeleteIgnoreNoExist(), Orphan: pb.GetOrphan()}
		return in
	}
	in.Spec.Update = &api.Update{
		Priority:          pb.GetPriority(),
		Updates:           pb.GetUpdate(),
		Deletes:           pb.GetDeletes(),
		RevertPaths:       pb.GetRevertPaths(),
		NonRevertive:      pb.GetNonRevertive(),
		PreviouslyApplied: pb.GetPreviouslyApplied(),
	}
	return in
}

func transactionResponse(res *api.TransactionResult) *sdcpb.TransactionSetResponse {
	if res == nil {
		return nil
	}
	out := &sdcpb.TransactionSetResponse{
		Update:   res.Updates,
		Delete:   res.Deletes,
		Warnings: res.Warnings,
		Intents:  make(map[string]*sdcpb.TransactionSetResponseIntent, len(res.Intents)),
	}
	for name, iv := range res.Intents {
		out.Intents[name] = &sdcpb.TransactionSetResponseIntent{Warnings: iv.Warnings, Errors: iv.Errors}
	}
	return out
}

func targetStatus(s api.ConnectionState) sdcpb.TargetStatus {
	switch s {
	case api.ConnectionConnected:
		return sdcpb.TargetStatus_CONNECTED
	case api.ConnectionNotConnected:
		return sdcpb.TargetStatus_NOT_CONNECTED
	default:
		return sdcpb.TargetStatus_UNKNOWN
	}
}

// deviationStream adapts a WatchDeviations stream to a datastore deviation sink.
type deviationStream struct {
	stream sdcpb.DataServer_WatchDeviationsServer
}

func (d *deviationStream) Context() context.Context { return d.stream.Context() }

func (d *deviationStream) Send(ev api.DeviationEvent) error {
	rsp := &sdcpb.WatchDeviationResponse{Name: ev.Datastore, Intent: ev.Intent, Reason: deviationReason(ev.Reason)}
	switch ev.Kind {
	case api.DeviationStart:
		rsp.Event = sdcpb.DeviationEvent_START
	case api.DeviationEnd:
		rsp.Event = sdcpb.DeviationEvent_END
	case api.DeviationClear:
		rsp.Event = sdcpb.DeviationEvent_CLEAR
	default:
		rsp.Event = sdcpb.DeviationEvent_UPDATE
		rsp.Path = ev.Path
		rsp.ExpectedValue = ev.Expected
		rsp.CurrentValue = ev.Current
	}
	return d.stream.Send(rsp)
}

func deviationReason(r api.DeviationReason) sdcpb.DeviationReason {
	switch r {
	case api.DeviationReasonUnhandled:
		return sdcpb.DeviationReason_UNHANDLED
	case api.DeviationReasonNotApplied:
		return sdcpb.DeviationReason_NOT_APPLIED
	case api.DeviationReasonOverruled:
		return sdcpb.DeviationReason_OVERRULED
	case api.DeviationReasonIntentExists:
		return sdcpb.DeviationReason_INTENT_EXISTS
	default:
		return sdcpb.DeviationReason_DR_UNKNOWN
	}
}
