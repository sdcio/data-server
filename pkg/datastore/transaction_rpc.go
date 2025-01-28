package datastore

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	if req.GetOrphan() {
		ti.SetDeleteOnlyIntendedFlag()
	}

	cacheUpdates, err := d.expandAndConvertIntent(ctx, req.GetIntent(), req.GetPriority(), req.GetUpdate())
	if err != nil {
		return nil, err
	}

	ti.updates = cacheUpdates

	return ti, nil
}

func (d *Datastore) transactionSet(ctx context.Context, transaction *Transaction, dryRun bool) (*sdcpb.TransactionSetResponse, error) {
	treeCacheSchemaClient := tree.NewTreeSchemaCacheClient(d.Name(), d.cacheClient, d.getValidationClient())
	tc := tree.NewTreeContext(treeCacheSchemaClient, d.Name())

	tc.GetTreeSchemaCacheClient().RefreshCaches(ctx)

	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}

	involvedPaths := tree.NewPathSet()

	for _, intent := range transaction.newIntents {
		tc.SetActualOwner(intent.name)

		log.Debugf("Transaction: %s - adding intent %s to tree", transaction.GetTransactionId(), intent.name)
		oldIntentContent, err := root.LoadIntendedStoreOwnerData(ctx, intent.name, intent.onlyIntended)
		if err != nil {
			return nil, err
		}

		err = transaction.AddIntentContent(intent.name, TransactionIntentOld, oldIntentContent.GetLowestPriorityValue(nil), oldIntentContent)
		if err != nil {
			return nil, err
		}

		flagsNew := tree.NewUpdateInsertFlags()
		flagsNew.SetNewFlag()

		err = root.AddCacheUpdatesRecursive(ctx, intent.updates, flagsNew)
		if err != nil {
			return nil, err
		}

		involvedPaths.Join(oldIntentContent.ToPathSet())
		involvedPaths.Join(intent.updates.ToPathSet())
	}

	err = root.LoadIntendedStoreHighestPrio(ctx, involvedPaths, transaction.GetIntentNames())
	if err != nil {
		return nil, err
	}

	err = d.populateTreeWithRunning(ctx, tc, root)
	if err != nil {
		return nil, err
	}

	log.Debugf("Transaction: %s - finish tree insertion phase", transaction.GetTransactionId())
	root.FinishInsertionPhase(ctx)

	result := &sdcpb.TransactionSetResponse{
		Intents:  map[string]*sdcpb.TransactionSetResponseIntent{},
		Update:   []*sdcpb.Update{},
		Delete:   []*sdcpb.Path{},
		Warnings: []string{},
	}

	// perform validation
	validationResult := root.Validate(ctx, true)

	for intentName, intentValidationResult := range validationResult {
		result.Intents[intentName] = &sdcpb.TransactionSetResponseIntent{
			Warnings: intentValidationResult.WarningsString(),
			Errors:   intentValidationResult.ErrorsString(),
		}
	}

	log.Infof("Transaction: %s - validation passed", transaction.GetTransactionId())

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

	log.Infof("ds=%s transaction=%s applied", d.Name(), transaction.GetTransactionId())

	/////////////////////////////////////
	// update intent in intended store //
	/////////////////////////////////////

	// logging
	strSl := tree.Map(updates.ToCacheUpdateSlice(), func(u *cache.Update) string { return u.String() })
	log.Debugf("Updates\n%s", strings.Join(strSl, "\n"))

	delSl := deletes.PathSlices()

	log.Debugf("Deletes:\n%s", strings.Join(strSl, "\n"))

	for _, intent := range transaction.newIntents {
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

	log.Infof("ds=%s transaction=%s: completed", d.Name(), transaction.GetTransactionId())
	err = transaction.StartRollbackTimer()
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (d *Datastore) TransactionSet(ctx context.Context, transactionId string, transactionTimeout time.Duration, dryRun bool, transactionIntents []*TransactionIntent) (*sdcpb.TransactionSetResponse, error) {
	var err error

	log.Infof("Transaction: %s - start", transactionId)

	transaction := NewTransaction(transactionId)
	transaction.SetTimeout(transactionTimeout, func() {
		d.TransactionCancel(context.Background(), transactionId)
	})

	registered := false
	for {
		select {
		case <-ctx.Done():
			// Context was canceled or timed out
			log.Errorf("Transaction: %s - context canceled or timed out: %v", transactionId, ctx.Err())
			return nil, ctx.Err()
		default:
			// Start a transaction and prepare to cancel it if any error occurs
			err = d.transactionManager.RegisterTransaction(ctx, transaction)
			if err == nil {
				registered = true
				break
			}
			log.Warnf("Transaction: %s - failed to create transaction, retrying: %v", transactionId, err)
			time.Sleep(time.Millisecond * 200)
		}
		if registered {
			break
		}
	}

	defer func() {
		// Cancel the transaction if it hasn't been committed (in case of an error)
		if registered {
			log.Infof("Transaction: %s - canceling due to error", transactionId)
			d.transactionManager.cleanupTransaction(transactionId)
		}
	}()

	err = transaction.AddTransactionIntents(transactionIntents, TransactionIntentNew)
	if err != nil {
		return nil, err
	}

	response, err := d.transactionSet(ctx, transaction, dryRun)
	if err != nil {
		return nil, err
	}

	// Mark the transaction as successfully committed
	registered = false // Prevent TransactionCancel() in defer
	return response, err
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
	return d.transactionManager.Confirm(transactionId)
}

func (d *Datastore) TransactionCancel(ctx context.Context, transactionId string) error {
	log.Infof("Transaction %s - Cancel", transactionId)
	return d.transactionManager.Cancel(ctx, transactionId)
}
