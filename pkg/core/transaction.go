package core

import (
	"context"
	"fmt"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/datastore"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/tree/consts"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// TransactionSet applies intents to the datastore named by ref. An empty
// opts.Timeout selects the configured default. Validation failures are
// reported in the result, not as an error. It fails with api.ErrDatastoreLocked
// while another action holds the datastore and with api.ErrNotConnected when
// the target is unreachable; both are retryable.
func (c *Core) TransactionSet(ctx context.Context, ref api.TxRef, opts api.SetOptions, intents []api.Intent) (*api.TransactionResult, error) {
	ctx, ds, err := c.txContext(ctx, "TransactionSet", ref, opts.Origin)
	if err != nil {
		return nil, err
	}
	logf.FromContext(ctx).Info("TransactionSet", "transaction-dry-run", opts.DryRun, "transaction-timeout", opts.Timeout)

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = c.cfg.DefaultTransactionTimeout
	}

	tis := make([]*types.TransactionIntent, 0, len(intents))
	for i := range intents {
		in := &intents[i]
		var ti *types.TransactionIntent
		switch {
		case in.Name == consts.RunningIntentName || (in.Spec.Update != nil && in.Spec.Update.Priority == consts.RunningValuesPrio):
			// only explicit deletes may target the running intent
			ti = types.NewTransactionIntent(consts.RevertRunningName, consts.RevertRunningPrio)
			if in.Spec.Update != nil {
				ti.AddExplicitDeletes(deletePaths(in.Spec.Update.Deletes))
			}
		case in.Spec.Update != nil && in.Spec.Update.Priority > consts.UserSettableMax:
			return nil, fmt.Errorf("%w: user settable priority not allowed above %d", api.ErrInvalidArgument, consts.UserSettableMax)
		default:
			pb, err := toSdcpbIntent(in)
			if err != nil {
				return nil, err
			}
			if ti, err = ds.SdcpbTransactionIntentToInternalTI(ctx, pb); err != nil {
				return nil, err
			}
		}
		tis = append(tis, ti)
	}

	var replace *types.TransactionIntent
	if opts.Replace != nil {
		pb, err := toSdcpbIntent(opts.Replace)
		if err != nil {
			return nil, err
		}
		pb.Priority = consts.ReplaceValuesPrio
		pb.Intent = consts.ReplaceIntentName
		if replace, err = ds.SdcpbTransactionIntentToInternalTI(ctx, pb); err != nil {
			return nil, err
		}
	}

	rsp, err := ds.TransactionSet(ctx, ref.ID, tis, replace, timeout, opts.DryRun)
	if err != nil {
		return nil, err
	}
	return toTransactionResult(rsp), nil
}

// TransactionConfirm confirms a transaction so it is not rolled back. It fails
// with api.ErrDatastoreLocked while another action holds the datastore.
func (c *Core) TransactionConfirm(ctx context.Context, ref api.TxRef, origin api.Origin) error {
	ctx, ds, err := c.txContext(ctx, "TransactionConfirm", ref, origin)
	if err != nil {
		return err
	}
	logf.FromContext(ctx).Info("TransactionConfirm")
	return ds.TransactionConfirm(ctx, ref.ID)
}

// TransactionCancel rolls a transaction back. It fails with
// api.ErrDatastoreLocked while another action holds the datastore.
func (c *Core) TransactionCancel(ctx context.Context, ref api.TxRef, origin api.Origin) error {
	ctx, ds, err := c.txContext(ctx, "TransactionCancel", ref, origin)
	if err != nil {
		return err
	}
	logf.FromContext(ctx).Info("TransactionCancel")
	return ds.TransactionCancel(ctx, ref.ID)
}

// txContext resolves the datastore of ref and returns ctx carrying the
// transaction logger.
func (c *Core) txContext(ctx context.Context, op string, ref api.TxRef, origin api.Origin) (context.Context, *datastore.Datastore, error) {
	ds, err := c.getDatastore(ref.Datastore)
	if err != nil {
		return nil, nil, err
	}
	log := logf.FromContext(ctx).WithName(op).WithName("transaction").WithValues(
		"transaction-id", ref.ID,
		"datastore-name", ref.Datastore,
		"transaction-origin", string(origin),
	)
	return logf.IntoContext(ctx, log), ds, nil
}

func toSdcpbIntent(in *api.Intent) (*sdcpb.TransactionIntent, error) {
	pb := &sdcpb.TransactionIntent{Intent: in.Name}
	switch {
	case in.Spec.Update != nil && in.Spec.Delete != nil:
		return nil, fmt.Errorf("%w: intent %q sets both update and delete", api.ErrInvalidArgument, in.Name)
	case in.Spec.Delete != nil:
		pb.Delete = true
		pb.DeleteIgnoreNoExist = in.Spec.Delete.IgnoreNoExist
		pb.Orphan = in.Spec.Delete.Orphan
	case in.Spec.Update != nil:
		u := in.Spec.Update
		pb.Priority = u.Priority
		pb.NonRevertive = u.NonRevertive
		pb.PreviouslyApplied = u.PreviouslyApplied
		pb.Deletes = deletePaths(u.Deletes)
		pb.RevertPaths = deletePaths(u.RevertPaths)
		for _, e := range u.Entries {
			path, err := sdcpb.ParsePath(e.Path)
			if err != nil {
				return nil, fmt.Errorf("%w: intent %q path %q: %v", api.ErrInvalidArgument, in.Name, e.Path, err)
			}
			pb.Update = append(pb.Update, &sdcpb.Update{
				Path:  path,
				Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: e.Content}},
			})
		}
	}
	return pb, nil
}

// deletePaths parses xpath strings; entries that do not parse are skipped.
func deletePaths(paths []string) []*sdcpb.Path {
	out := make([]*sdcpb.Path, 0, len(paths))
	for _, p := range paths {
		if path, err := sdcpb.ParsePath(p); err == nil {
			out = append(out, path)
		}
	}
	return out
}

func toTransactionResult(rsp *sdcpb.TransactionSetResponse) *api.TransactionResult {
	res := &api.TransactionResult{
		Warnings: rsp.GetWarnings(),
		Intents:  make(map[string]api.IntentValidation, len(rsp.GetIntents())),
	}
	for _, u := range rsp.GetUpdate() {
		res.Updates = append(res.Updates, api.PathValue{Path: u.GetPath().ToXPath(false), Value: valueString(u.GetValue())})
	}
	for _, d := range rsp.GetDelete() {
		res.Deletes = append(res.Deletes, d.ToXPath(false))
	}
	for name, iv := range rsp.GetIntents() {
		res.Intents[name] = api.IntentValidation{Warnings: iv.GetWarnings(), Errors: iv.GetErrors()}
	}
	return res
}

func valueString(tv *sdcpb.TypedValue) string {
	if tv == nil {
		return ""
	}
	return tv.ToString()
}
