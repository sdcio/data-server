package grpcserver

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func schemaRef(s *sdcpb.Schema) api.SchemaRef {
	return api.SchemaRef{Name: s.GetName(), Vendor: s.GetVendor(), Version: s.GetVersion()}
}

func sdcpbSchema(r api.SchemaRef) *sdcpb.Schema {
	return &sdcpb.Schema{Name: r.Name, Vendor: r.Vendor, Version: r.Version}
}

// datastoreSpec converts the target, schema and sync of a create request. The
// returned errors are gRPC statuses carrying the messages clients already see.
func datastoreSpec(req *sdcpb.CreateDataStoreRequest) (api.DatastoreSpec, error) {
	target, err := target(req.GetTarget())
	if err != nil {
		return api.DatastoreSpec{}, err
	}
	spec := api.DatastoreSpec{Name: req.GetDatastoreName(), Target: target, Schema: schemaRef(req.GetSchema())}
	if req.GetSync() != nil {
		if spec.Sync, err = syncConfig(req.GetSync()); err != nil {
			return api.DatastoreSpec{}, err
		}
	}
	return spec, nil
}

func target(t *sdcpb.Target) (api.Target, error) {
	out := api.Target{Type: api.TargetType(t.GetType()), Address: t.GetAddress(), Port: t.GetPort()}
	switch strings.ToLower(t.GetType()) {
	case "netconf":
		nc := t.GetNetconfOpts()
		var commit api.CommitCandidate
		switch nc.GetCommitCandidate() {
		case sdcpb.CommitCandidate_COMMIT_CANDIDATE:
			commit = api.CommitCandidateCandidate
		case sdcpb.CommitCandidate_COMMIT_RUNNING:
			commit = api.CommitCandidateRunning
		default:
			return api.Target{}, fmt.Errorf("unknown commitDatastore: %v", nc.GetCommitCandidate())
		}
		out.ProtocolOptions.Netconf = &api.NetconfOptions{
			IncludeNs:          nc.GetIncludeNs(),
			OperationWithNs:    nc.GetOperationWithNs(),
			UseOperationRemove: nc.GetUseOperationRemove(),
			CommitCandidate:    commit,
		}
	case "gnmi":
		g := t.GetGnmiOpts()
		out.ProtocolOptions.GNMI = &api.GNMIOptions{Encoding: api.GNMIEncoding(g.GetEncoding()), TargetName: g.GetTargetName()}
	case "noop":
	default:
		return api.Target{}, status.Errorf(codes.InvalidArgument, "unknown targetconnection protocol type %q", t.GetType())
	}
	if tls := t.GetTls(); tls != nil {
		out.TLS = &api.TLS{CA: tls.GetCa(), Cert: tls.GetCert(), Key: tls.GetKey(), SkipVerify: tls.GetSkipVerify()}
	}
	if c := t.GetCredentials(); c != nil {
		out.Credentials = &api.Credentials{Username: c.GetUsername(), Password: c.GetPassword(), Token: c.GetToken()}
	}
	return out, nil
}

func syncConfig(s *sdcpb.Sync) (api.Sync, error) {
	out := api.Sync{}
	for _, ps := range s.GetConfig() {
		typ := ps.GetTarget().GetType()
		tgt := api.Target{Type: api.TargetType(typ)}
		switch strings.ToLower(typ) {
		case "gnmi":
			tgt.ProtocolOptions.GNMI = &api.GNMIOptions{Encoding: api.GNMIEncoding(ps.GetTarget().GetGnmiOpts().GetEncoding())}
		case "netconf":
		default:
			return api.Sync{}, status.Errorf(codes.InvalidArgument, "unknown targetsync protocol: %q", typ)
		}
		out.Configs = append(out.Configs, api.SyncConfig{
			Name:     ps.GetName(),
			Target:   &tgt,
			Paths:    ps.GetPath(),
			Mode:     syncMode(ps.GetMode()),
			Interval: ps.GetInterval(),
		})
	}
	return out, nil
}

func syncMode(m sdcpb.SyncMode) api.SyncMode {
	switch m {
	case sdcpb.SyncMode_SM_SAMPLE:
		return api.SyncModeSample
	case sdcpb.SyncMode_SM_ONCE:
		return api.SyncModeOnce
	case sdcpb.SyncMode_SM_GET:
		return api.SyncModeGet
	default:
		return api.SyncModeOnChange
	}
}

func datastoreResponse(info *api.DatastoreInfo) *sdcpb.GetDataStoreResponse {
	return &sdcpb.GetDataStoreResponse{
		DatastoreName: info.Name,
		Intents:       info.Intents,
		Target: &sdcpb.Target{
			Type:          string(info.Spec.Target.Type),
			Address:       info.Spec.Target.Address,
			Status:        targetStatus(info.ConnectionState),
			StatusDetails: info.StatusDetails,
		},
		Schema: sdcpbSchema(info.Spec.Schema),
	}
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

// intent converts a transaction intent. The running intent is always an
// update carrying only explicit deletes, whatever its delete flag says.
func intent(pb *sdcpb.TransactionIntent) (api.Intent, error) {
	in := api.Intent{Name: pb.GetIntent()}
	running := pb.GetPriority() == consts.RunningValuesPrio || pb.GetIntent() == consts.RunningIntentName
	if pb.GetDelete() && !running {
		in.Spec.Delete = &api.Delete{IgnoreNoExist: pb.GetDeleteIgnoreNoExist(), Orphan: pb.GetOrphan()}
		return in, nil
	}
	u := &api.Update{
		Priority:          pb.GetPriority(),
		NonRevertive:      pb.GetNonRevertive(),
		PreviouslyApplied: pb.GetPreviouslyApplied(),
	}
	for _, d := range pb.GetDeletes() {
		u.Deletes = append(u.Deletes, d)
	}
	for _, p := range pb.GetRevertPaths() {
		u.RevertPaths = append(u.RevertPaths, p)
	}
	u.Updates = pb.GetUpdate()
	in.Spec.Update = u
	return in, nil
}

func setOptions(req *sdcpb.TransactionSetRequest, origin api.Origin) (api.SetOptions, error) {
	opts := api.SetOptions{DryRun: req.GetDryRun(), Origin: origin}
	if req.Timeout != nil {
		opts.Timeout = time.Duration(*req.Timeout) * time.Second
	}
	if req.GetReplaceIntent() != nil {
		rep, err := intent(req.GetReplaceIntent())
		if err != nil {
			return api.SetOptions{}, err
		}
		opts.Replace = &rep
	}
	return opts, nil
}

func transactionResponse(res *api.TransactionResult) *sdcpb.TransactionSetResponse {
	out := &sdcpb.TransactionSetResponse{
		Warnings: res.Warnings,
		Intents:  make(map[string]*sdcpb.TransactionSetResponseIntent, len(res.Intents)),
	}
	for _, u := range res.Updates {
		out.Update = append(out.Update, &sdcpb.Update{Path: u.Path, Value: u.Value})
	}
	out.Delete = append(out.Delete, res.Deletes...)
	for name, iv := range res.Intents {
		out.Intents[name] = &sdcpb.TransactionSetResponseIntent{Warnings: iv.Warnings, Errors: iv.Errors}
	}
	return out
}

func intentFormat(f sdcpb.Format) (api.Format, error) {
	switch f {
	case sdcpb.Format_Intent_Format_JSON:
		return api.FormatJSON, nil
	case sdcpb.Format_Intent_Format_JSON_IETF:
		return api.FormatJSONIETF, nil
	case sdcpb.Format_Intent_Format_XML:
		return api.FormatXML, nil
	case sdcpb.Format_Intent_Format_PROTO:
		return api.FormatProto, nil
	case sdcpb.Format_Intent_Format_XPATH:
		return api.FormatXPath, nil
	default:
		return 0, errors.New("unknown format")
	}
}

func intentResponse(req *sdcpb.GetIntentRequest, res *api.IntentResult) (*sdcpb.GetIntentResponse, error) {
	out := &sdcpb.GetIntentResponse{
		DatastoreName: req.GetDatastoreName(),
		Format:        req.GetFormat(),
		IntentName:    req.GetIntent(),
		Orphan:        res.Orphan,
		Priority:      res.Priority,
		NonRevertive:  res.NonRevertive,
	}
	out.Deletes = append(out.Deletes, res.Deletes...)
	if res.Format == api.FormatProto {
		var in sdcpb.Intent
		if err := proto.Unmarshal(res.Content, &in); err != nil {
			return nil, err
		}
		out.Intent = &sdcpb.GetIntentResponse_Proto{Proto: &in}
		return out, nil
	}
	out.Intent = &sdcpb.GetIntentResponse_Blob{Blob: res.Content}
	return out, nil
}

func blameTree(n *api.BlameNode) *sdcpb.BlameTreeElement {
	if n == nil {
		return nil
	}
	e := &sdcpb.BlameTreeElement{Name: n.Name, Owner: n.Owner, KeyName: n.KeyName}
	e.Value = n.Value
	e.DeviationValue = n.DeviationValue
	for _, ch := range n.Children {
		e.Childs = append(e.Childs, blameTree(ch))
	}
	return e
}

func schemaDetails(info *api.SchemaInfo) *sdcpb.GetSchemaDetailsResponse {
	st := sdcpb.SchemaStatus_OK
	switch info.State {
	case api.SchemaReloading:
		st = sdcpb.SchemaStatus_RELOADING
	case api.SchemaInitializing:
		st = sdcpb.SchemaStatus_INITIALIZING
	case api.SchemaFailed:
		st = sdcpb.SchemaStatus_FAILED
	}
	s := sdcpbSchema(info.Schema)
	s.Status = st
	return &sdcpb.GetSchemaDetailsResponse{Schema: s, File: info.Files, Directory: info.Directories, Exclude: info.Excludes}
}

func sdcpbDeviationReason(r api.DeviationReason) sdcpb.DeviationReason {
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

func deviationResponse(ev api.DeviationEvent) *sdcpb.WatchDeviationResponse {
	rsp := &sdcpb.WatchDeviationResponse{Name: ev.Datastore, Intent: ev.Intent, Reason: sdcpbDeviationReason(ev.Reason)}
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
	return rsp
}
