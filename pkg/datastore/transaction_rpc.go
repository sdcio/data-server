package datastore

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func (d *Datastore) expandAndConvertIntent(ctx context.Context, intentName string, priority int32, upds []*sdcpb.Update) (tree.UpdateSlice, error) {
	converter := utils.NewConverter(d.getValidationClient())

	// list of updates to be added to the cache
	// Expands the value, in case of json to single typed value updates
	expandedReqUpdates, err := converter.ExpandUpdates(ctx, upds, true)
	if err != nil {
		return nil, err
	}

	// temp storage for cache.Update of the req. They are to be added later.
	newCacheUpdates := make([]*cache.Update, 0, len(expandedReqUpdates))

	for _, u := range expandedReqUpdates {
		pathslice, err := utils.CompletePath(nil, u.GetPath())
		if err != nil {
			return nil, err
		}

		// since we already have the pathslice, we construct the cache.Update, but keep it for later
		// addition to the tree. First we need to mark the existing once for deletion

		// make sure typedValue is carrying the correct type
		err = d.validateUpdate(ctx, u)
		if err != nil {
			return nil, err
		}

		// convert value to []byte for cache insertion
		val, err := proto.Marshal(u.GetValue())
		if err != nil {
			return nil, err
		}

		// construct the cache.Update
		newCacheUpdates = append(newCacheUpdates, cache.NewUpdate(pathslice, val, priority, intentName, 0))
	}
	return newCacheUpdates, nil
}

func (d *Datastore) SdcpbTransactionIntentToInternalTI(ctx context.Context, req *sdcpb.TransactionIntent) (*TransactionIntent, error) {

	ti := NewTransactionIntent(req.GetIntent(), req.GetPriority())
	if req.GetDelete() {
		ti.SetDeleteFlag()
	}
	if req.GetOnlyIntended() {
		ti.SetDeleteOnlyIntendedFlag()
	}

	cacheUpdates, err := d.expandAndConvertIntent(ctx, req.GetIntent(), req.GetPriority(), req.GetUpdate())
	if err != nil {
		return nil, err
	}

	ti.updates = cacheUpdates

	return ti, nil
}

func (d *Datastore) transactionSet(ctx context.Context, recordTransaction *Transaction, dryRun bool, setTransaction *Transaction) (*sdcpb.TransactionSetResponse, error) {
	treeCacheSchemaClient := tree.NewTreeSchemaCacheClient(d.Name(), d.cacheClient, d.getValidationClient())
	tc := tree.NewTreeContext(treeCacheSchemaClient, d.Name())

	tc.GetTreeSchemaCacheClient().RefreshCaches(ctx)

	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}

	involvedPaths := &tree.PathSet{}

	for _, intent := range setTransaction.intents {
		tc.SetActualOwner(intent.name)

		log.Debugf("Transaction: %s - adding intent %s to tree", recordTransaction.GetTransactionId(), intent.name)
		oldIntentContent, err := root.LoadIntendedStoreOwnerData(ctx, intent.name, intent.onlyIntended)
		if err != nil {
			return nil, err
		}

		err = recordTransaction.AddIntentContent(intent.name, oldIntentContent.GetLowestPriorityValue(nil), oldIntentContent)
		if err != nil {
			return nil, err
		}

		err = d.populateTree(ctx, root, intent.updates)
		if err != nil {
			return nil, err
		}

		involvedPaths.Join(oldIntentContent.ToPathSet())
		involvedPaths.Join(intent.updates.ToPathSet())
	}

	err = root.LoadIntendedStoreHighestPrio(ctx, involvedPaths, setTransaction.GetIntentNames())
	if err != nil {
		return nil, err
	}

	err = d.populateTreeWithRunning(ctx, tc, root)
	if err != nil {
		return nil, err
	}

	log.Debugf("Transaction: %s - finish tree insertion phase", recordTransaction.GetTransactionId())
	root.FinishInsertionPhase(ctx)

	result := &sdcpb.TransactionSetResponse{}

	// perform validation
	validationResult := root.Validate(ctx, true)

	for intentName, intentValidationResult := range validationResult {
		result.Intents[intentName] = &sdcpb.TransactionSetResponseIntent{
			Warnings: intentValidationResult.WarningsString(),
			Errors:   intentValidationResult.ErrorsString(),
		}
	}

	log.Infof("Transaction: %s - validation passed", recordTransaction.GetTransactionId())

	// retrieve the data that is meant to be send southbound (towards the device)
	updates := root.GetHighestPrecedence(true)
	deletes, err := root.GetDeletes(true)
	if err != nil {
		return nil, err
	}

	result.Update, err = cacheUpdateToSdcpbUpdate(updates)
	if err != nil {
		return nil, err
	}

	// add all the deletes to the setDataReq
	for _, u := range deletes {
		p, err := u.SdcpbPath()
		if err != nil {
			return nil, err
		}
		result.Delete = append(result.Delete, p)
	}

	// if it is a dry run, return now, skipping updating the device or the cache
	if dryRun {
		return result, nil
	}

	// apply the resulting config to the device
	dataResp, err := d.applyIntent(ctx, root)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, dataResp.GetWarnings()...)

	log.Infof("ds=%s transaction=%s applied", d.Name(), recordTransaction.GetTransactionId())

	/////////////////////////////////////
	// update intent in intended store //
	/////////////////////////////////////

	// logging
	strSl := tree.Map(updates.ToCacheUpdateSlice(), func(u *cache.Update) string { return u.String() })
	log.Debugf("Updates\n%s", strings.Join(strSl, "\n"))

	delSl := deletes.PathSlices()

	log.Debugf("Deletes:\n%s", strings.Join(strSl, "\n"))

	for _, intent := range setTransaction.intents {
		// retrieve the data that is meant to be send towards the cache
		updatesOwner := root.GetUpdatesForOwner(intent.name)
		deletesOwner := root.GetDeletesForOwner(intent.name)

		// logging
		strSl := tree.Map(updatesOwner, func(u *cache.Update) string { return u.String() })
		log.Debugf("Updates Owner:\n%s", strings.Join(strSl, "\n"))

		strSl = deletesOwner.StringSlice()
		log.Debugf("Deletes Owner:\n%s", strings.Join(strSl, "\n"))

		// modify intended store per intent
		err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
			Store:    cachepb.Store_INTENDED,
			Owner:    intent.name,
			Priority: intent.priority,
		}, deletesOwner.ToStringSlice(), updatesOwner)

		if err != nil {
			return nil, fmt.Errorf("failed updating the intended store for %s: %w", d.Name(), err)
		}
	}

	// fast and optimistic writeback to the config store
	err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
		Store: cachepb.Store_CONFIG,
	}, delSl.ToStringSlice(), updates.ToCacheUpdateSlice())
	if err != nil {
		return nil, fmt.Errorf("failed updating the running config store for %s: %w", d.Name(), err)
	}

	log.Infof("ds=%s transaction=%s: completet", d.Name(), recordTransaction.GetTransactionId())

	return nil, nil
}

func (d *Datastore) TransactionSet(ctx context.Context, transactionId string, dryRun bool, intents []*sdcpb.TransactionIntent) (*sdcpb.TransactionSetResponse, error) {

	log.Infof("Transaction: %s - start", transactionId)
	recordTransaction := d.transactionManager.CreateTransaction(transactionId)

	setTransaction := NewTransaction(transactionId)

	for _, intent := range intents {
		us, err := d.expandAndConvertIntent(ctx, intent.GetIntent(), intent.GetPriority(), intent.GetUpdate())
		if err != nil {
			return nil, err
		}
		err = setTransaction.AddIntentContent(intent.GetIntent(), intent.GetPriority(), us)
		if err != nil {
			return nil, err
		}

	}

	return d.transactionSet(ctx, recordTransaction, dryRun, setTransaction)
}

func cacheUpdateToSdcpbUpdate(lvs tree.LeafVariantSlice) ([]*sdcpb.Update, error) {
	result := make([]*sdcpb.Update, 0, len(lvs))
	for _, lv := range lvs {
		path, err := lv.GetEntry().SdcpbPath()
		if err != nil {
			return nil, err
		}
		value, err := lv.Update.Value()
		if err != nil {
			return nil, err
		}
		upd := &sdcpb.Update{
			Path:  path,
			Value: value,
		}
		result = append(result, upd)
	}
	return result, nil
}

func (d *Datastore) TransactionConfirm(ctx context.Context, transactionId string) error {
	log.Infof("Transaction %s - Confirm", transactionId)
	// everything remains as is
	return d.transactionManager.FinishTransaction(transactionId)
}

func (d *Datastore) TransactionCancel(ctx context.Context, transactionId string) error {
	log.Infof("Transaction %s - Cancel", transactionId)

	trans := NewTransaction(fmt.Sprintf("%s - Rollback", transactionId))

	oldTrans, err := d.transactionManager.GetTransaction(transactionId)
	if err != nil {
		return err
	}

	_, err = d.transactionSet(ctx, trans, false, oldTrans)
	if err != nil {
		return err
	}

	return d.transactionManager.FinishTransaction(transactionId)
}
