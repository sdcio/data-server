package datastore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/tree"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	log "github.com/sirupsen/logrus"
)

var (
	ErrDatastoreLocked   = errors.New("datastore is locked, other action is ongoing")
	ErrContextDone       = errors.New("context is closed (done)")
	ErrValidationError   = errors.New("validation error")
	ErrNoIntentsProvided = errors.New("no intents provided")
)

// SdcpbTransactionIntentToInternalTI converts sdcpb.TransactionIntent to types.TransactionIntent
func (d *Datastore) SdcpbTransactionIntentToInternalTI(ctx context.Context, req *sdcpb.TransactionIntent) (*types.TransactionIntent, error) {

	// create a new TransactionIntent with the given name and priority
	ti := types.NewTransactionIntent(req.GetIntent(), req.GetPriority())

	// setting the provided flags in the TransactionIntent
	if req.GetDelete() {
		ti.SetDeleteFlag()
	}
	if req.GetOrphan() {
		ti.SetDeleteOnlyIntendedFlag()
	}
	if req.GetDeviation() {
		ti.SetDeviation()
	}
	if req.GetDeleteIgnoreNoExist() {
		ti.SetDeleteIgnoreNonExisting()
	}

	// convert the sdcpb.updates to tree.UpdateSlice
	Updates, err := treetypes.ExpandAndConvertIntent(ctx, d.schemaClient, req.GetIntent(), req.GetPriority(), req.GetUpdate(), time.Now().Unix())
	if err != nil {
		return nil, err
	}

	// add the intent to the TransactionIntent
	ti.AddUpdates(Updates)

	// add the deletes
	ti.AddExplicitDeletes(req.Deletes)

	return ti, nil
}

// replaceIntent takes a Transaction and treats it as a replaceIntent, replacing the whole device configuration with the content of the given intent.
// returns the warnings as a []string and potential errors that happend during validation / from SBI Set()
func (d *Datastore) replaceIntent(ctx context.Context, transaction *types.Transaction) ([]string, error) {

	// create a new TreeContext
	tc := tree.NewTreeContext(d.schemaClient, d.Name())

	// create a new TreeRoot to collect validate and hand to SBI.Set()
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}

	// set TreeContext actual owner to the const of ReplaceIntentName
	tc.SetActualOwner(tree.ReplaceIntentName)

	// store the actual / old running in the transaction
	runningProto, err := d.cacheClient.IntentGet(ctx, tree.RunningIntentName)
	if err != nil {
		return nil, err
	}
	err = root.ImportConfig(ctx, nil, treeproto.NewProtoTreeImporter(runningProto), tree.RunningIntentName, tree.RunningValuesPrio, treetypes.NewUpdateInsertFlags())
	if err != nil {
		return nil, err
	}

	// creat a InsertFlags struct with the New flag set.
	flagNew := treetypes.NewUpdateInsertFlags()
	flagNew.SetNewFlag()

	// add all the replace transaction updates with the New flag set
	err = root.AddUpdatesRecursive(ctx, transaction.GetReplace().GetUpdates(), flagNew)
	if err != nil {
		return nil, err
	}

	log.Debugf("Transaction Replace: %s - finish tree insertion phase", transaction.GetTransactionId())
	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}

	// log the tree in trace level, making it a func call to spare overhead in lower log levels.
	log.TraceFn(func() []interface{} { return []interface{}{root.String()} })

	// perform validation
	validationResult, validationStats := root.Validate(ctx, d.config.Validation)
	validationResult.ErrorsStr()
	if validationResult.HasErrors() {
		return nil, validationResult.JoinErrors()
	}

	warnings := validationResult.WarningsStr()
	log.Debugf("Transaction: %s - validation stats - %s", transaction.GetTransactionId(), validationStats.String())
	log.Infof("Transaction: %s - validation passed", transaction.GetTransactionId())

	// we use the TargetSourceReplace, that adjustes the tree results in a way
	// that the whole config tree is getting replaced.
	replaceRoot := types.NewTargetSourceReplace(root)

	// apply the resulting config to the device
	dataResp, err := d.applyIntent(ctx, replaceRoot)
	if err != nil {
		return nil, err
	}
	// collect warnings
	warnings = append(warnings, dataResp.GetWarnings()...)

	log.Infof("ds=%s transaction=%s applied", d.Name(), transaction.GetTransactionId()+" - replace")

	// retrieve the data that is meant to be send southbound (towards the device)
	updates := root.GetHighestPrecedence(true)
	deletes := treetypes.DeleteEntriesList{root}

	// OPTIMISTIC WRITEBACK TO RUNNING / syncTree
	err = d.writeBackSyncTree(ctx, updates, deletes)
	if err != nil {
		return nil, err
	}

	return warnings, nil
}

func (d *Datastore) LoadAllButRunningIntents(ctx context.Context, root *tree.RootEntry, excludeDeviations bool) ([]string, error) {

	intentNames := []string{}
	IntentChan := make(chan *tree_persist.Intent)
	ErrChan := make(chan error, 1)

	go d.cacheClient.IntentGetAll(ctx, []string{"running"}, IntentChan, ErrChan)

	for {
	selectLoop:
		select {
		case err, ok := <-ErrChan:
			if !ok {
				// ErrChan already closed which is fine, continue
				ErrChan = nil
				break selectLoop
			}
			return nil, err
		case <-ctx.Done():
			return nil, fmt.Errorf("context closed while retrieving all intents")
		case intent, ok := <-IntentChan:
			if !ok {
				// IntentChan closed due to finish
				IntentChan = nil
				break selectLoop
			}
			if excludeDeviations && intent.Deviation {
				continue
			}
			intentNames = append(intentNames, intent.GetIntentName())
			log.Debugf("%s: adding intent %s to tree", d.Name(), intent.GetIntentName())
			protoLoader := treeproto.NewProtoTreeImporter(intent)
			log.Tracef("%s", intent.String())
			err := root.ImportConfig(ctx, nil, protoLoader, intent.GetIntentName(), intent.GetPriority(), treetypes.NewUpdateInsertFlags())
			if err != nil {
				return nil, err
			}
		}
		if ErrChan == nil && IntentChan == nil {
			return intentNames, nil
		}
	}
}

// lowlevelTransactionSet
func (d *Datastore) lowlevelTransactionSet(ctx context.Context, transaction *types.Transaction, dryRun bool) (*sdcpb.TransactionSetResponse, error) {
	// create a new TreeRoot
	d.syncTreeMutex.Lock()
	root, err := d.syncTree.DeepCopy(ctx)
	d.syncTreeMutex.Unlock()
	if err != nil {
		return nil, err
	}

	_, err = d.LoadAllButRunningIntents(ctx, root, false)
	if err != nil {
		return nil, err
	}

	// create a flags attribute
	flagNew := treetypes.NewUpdateInsertFlags()
	// where the New flag is set
	flagNew.SetNewFlag()

	// iterate through all the intents
	for _, intent := range transaction.GetNewIntents() {
		// update the TreeContext to reflect the actual owner (intent name)
		lvs := tree.LeafVariantSlice{}
		lvs = root.GetByOwner(intent.GetName(), lvs)

		oldIntentContent := lvs.ToUpdateSlice()

		marksOwnerDeleteVisitor := tree.NewMarkOwnerDeleteVisitor(intent.GetName(), intent.GetOnlyIntended())
		err := root.Walk(ctx, marksOwnerDeleteVisitor)
		if err != nil {
			return nil, err
		}
		// clear the owners existing explicit delete entries, retrieving the old entries for storing in the transaction for possible rollback
		oldExplicitDeletes := root.RemoveExplicitDeletes(intent.GetName())

		// store the old intent content in the transaction as the old intent.
		err = transaction.AddIntentContent(intent.GetName(), types.TransactionIntentOld, oldIntentContent.GetFirstPriorityValue(), oldIntentContent, oldExplicitDeletes)
		if err != nil {
			return nil, err
		}

		if !intent.GetDeleteFlag() {
			// add the content to the Tree
			err = root.AddUpdatesRecursive(ctx, intent.GetUpdates(), flagNew)
			if err != nil {
				return nil, err
			}

			// add the explicit delete entries
			root.AddExplicitDeletes(intent.GetName(), intent.GetPriority(), intent.GetDeletes())
		}
	}

	les := tree.LeafVariantSlice{}
	les = root.GetByOwner(tree.RunningIntentName, les)

	transaction.GetOldRunning().AddUpdates(les.ToUpdateSlice())

	log.Debugf("Transaction: %s - finish tree insertion phase", transaction.GetTransactionId())
	// FinishInsertion Phase
	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}

	log.Debug(root.String())

	// perform validation
	validationResult, validationStats := root.Validate(ctx, d.config.Validation)

	log.Debugf("Transaction: %s - Validation Stats: %s", transaction.GetTransactionId(), validationStats.String())

	// prepare the response struct
	result := &sdcpb.TransactionSetResponse{
		Intents:  map[string]*sdcpb.TransactionSetResponseIntent{},
		Update:   []*sdcpb.Update{},
		Delete:   []*sdcpb.Path{},
		Warnings: []string{},
	}

	// process the validation results, adding them to the response
	for intentName, intentValidationResult := range validationResult {
		result.Intents[intentName] = &sdcpb.TransactionSetResponseIntent{
			Warnings: intentValidationResult.WarningsString(),
			Errors:   intentValidationResult.ErrorsString(),
		}
	}

	// retrieve the data that is meant to be send southbound (towards the device)
	updates := root.GetHighestPrecedence(true)
	deletes, err := root.GetDeletes(true)
	if err != nil {
		return nil, err
	}

	// convert updates from cache.Update to sdcpb.Update
	// adding them to the response
	result.Update, err = updateToSdcpbUpdate(updates)
	if err != nil {
		return nil, err
	}

	// add all the deletes to the setDataReq
	for _, u := range deletes {
		result.Delete = append(result.Delete, u.SdcpbPath())
	}

	// Error out if validation failed.
	if validationResult.HasErrors() {
		return result, ErrValidationError
	}

	log.Infof("Transaction: %s - validation passed", transaction.GetTransactionId())

	// if it is a dry run, return now, skipping updating the device or the cache
	if dryRun {
		log.Infof("Transaction: %s - dryrun finished successfull", transaction.GetTransactionId())
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
	updStrSl := treetypes.Map(updates.ToUpdateSlice(), func(u *treetypes.Update) string { return u.String() })
	log.Debugf("Updates:\n%s", strings.Join(updStrSl, "\n"))
	log.Debugf("Deletes:\n%s", strings.Join(deletes.SdcpbPaths().ToXPathSlice(), "\n"))

	for _, intent := range transaction.GetNewIntents() {
		// retrieve the data that is meant to be send towards the cache
		updatesOwner := root.GetUpdatesForOwner(intent.GetName())
		deletesOwner := root.GetDeletesForOwner(intent.GetName())

		// logging
		strSl := treetypes.Map(updatesOwner, func(u *treetypes.Update) string { return u.String() })
		log.Debugf("Updates Owner: %s\n%s", intent.GetName(), strings.Join(strSl, "\n"))

		delSl := deletesOwner.ToXPathSlice()
		log.Debugf("Deletes Owner: %s\n%s", intent.GetName(), strings.Join(delSl, "\n"))

		protoIntent, err := root.TreeExport(intent.GetName(), intent.GetPriority(), intent.Deviation())
		switch {
		case errors.Is(err, tree.ErrorIntentNotPresent):
			err = d.cacheClient.IntentDelete(ctx, intent.GetName(), intent.GetDeleteIgnoreNonExisting())
			if err != nil {
				log.Warnf("failed deleting intent from store for %s: %v", d.Name(), err)
			}
			continue
		case err != nil:
			return nil, err
		}
		err = d.cacheClient.IntentModify(ctx, protoIntent)
		if err != nil {
			return nil, fmt.Errorf("failed updating the intended store for %s: %w", d.Name(), err)
		}
	}

	// OPTIMISTIC WRITEBACK TO RUNNING / syncTree
	err = d.writeBackSyncTree(ctx, updates, deletes)
	if err != nil {
		return nil, err
	}

	log.Infof("ds=%s transaction=%s: completed", d.Name(), transaction.GetTransactionId())
	// start the rollback ticker only if it was not already a rollback transaction.
	if !transaction.IsRollback() {
		// start the RollbackTimer
		err = transaction.StartRollbackTimer()
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// writeBackSyncTree applies the provided changes to the syncTree and applies to the running cache intent
func (d *Datastore) writeBackSyncTree(ctx context.Context, updates tree.LeafVariantSlice, deletes treetypes.DeleteEntriesList) error {
	runningUpdates := updates.ToUpdateSlice().CopyWithNewOwnerAndPrio(tree.RunningIntentName, tree.RunningValuesPrio)

	// lock the syncTree
	d.syncTreeMutex.Lock()

	// perform deletes
	err := d.syncTree.DeleteBranchPaths(ctx, deletes, tree.RunningIntentName)
	if err != nil {
		return err
	}

	// add the calculated updates to the tree, as running with adjusted prio and owner
	err = d.syncTree.AddUpdatesRecursive(ctx, runningUpdates, treetypes.NewUpdateInsertFlags())
	if err != nil {
		return err
	}

	// release the syncTree lock
	d.syncTreeMutex.Unlock()

	// export the synctree
	newRunningIntent, err := d.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
	if err != nil {
		return err
	}
	// write the synctree to disk
	if newRunningIntent != nil {
		err = d.cacheClient.IntentModify(ctx, newRunningIntent)
		if err != nil {
			return fmt.Errorf("failed updating the running store for %s: %w", d.Name(), err)
		}
	}
	return nil
}

func (d *Datastore) TransactionSet(ctx context.Context, transactionId string, transactionIntents []*types.TransactionIntent, replaceIntent *types.TransactionIntent, transactionTimeout time.Duration, dryRun bool) (*sdcpb.TransactionSetResponse, error) {
	var err error
	var transaction *types.Transaction
	var transactionGuard *types.TransactionGuard

	log.Infof("Transaction: %s - start", transactionId)

	// create a new Transaction with the given transaction id
	transaction = types.NewTransaction(transactionId, d.transactionManager)
	// set the timeout on the transaction
	transaction.SetTimeout(transactionTimeout)

	if !dryRun {
		// try locking the datastore if it is locked return the specific ErrDatastoreLocked error.
		if !d.dmutex.TryLock() {
			log.Infof("Transaction: %s - abort (%v)", transactionId, ErrDatastoreLocked)
			return nil, ErrDatastoreLocked
		}
		defer d.dmutex.Unlock()

		// Try to register the Transaction in the TransactionManager only a single transaction can be register (implicitly being active)
	outerloop:
		for {
			select {
			case <-ctx.Done():
				// Context was canceled or timed out
				log.Errorf("Transaction: %s - context canceled or timed out: %v", transactionId, ctx.Err())
				return nil, ErrContextDone
			default:
				// Start a transaction and prepare to cancel it if any error occurs
				transactionGuard, err = d.transactionManager.RegisterTransaction(ctx, transaction)
				if err != nil {
					return nil, err
				}
				if transactionGuard != nil {
					defer transactionGuard.Done()
					break outerloop
				}
				return nil, ErrDatastoreLocked
			}
		}
	}

	// add the replaceIntent to the transaction
	transaction.SetReplace(replaceIntent)

	// if replace intent is provided, kickoff the replace intent processing first
	if transaction.GetReplace() != nil {
		replaceWarn, err := d.replaceIntent(ctx, transaction)
		if err != nil {
			log.Errorf("error setting replace intent: %v", err)
			return nil, err
		}
		// TODO: do something with these warnings
		_ = replaceWarn
	}

	err = transaction.AddTransactionIntents(transactionIntents, types.TransactionIntentNew)
	if err != nil {
		log.Errorf("error adding intents to transaction: %v", err)
		return nil, err
	}

	// no-op transaction
	if transaction.IsNoOp() {
		// we expect a transaction confirm from the client
		transactionGuard.Success()
		return &sdcpb.TransactionSetResponse{
			Warnings: []string{"no intents provided"},
		}, nil
	}

	response, err := d.lowlevelTransactionSet(ctx, transaction, dryRun)
	// if it is a validation error, we need to send the response while not successing the transaction guard
	// since validation errors are transported in the response itself, not in the seperate error
	if errors.Is(err, ErrValidationError) {
		log.Errorf("Transaction: %s - validation failed\n%s", transactionId, strings.Join(response.GetErrors(), "\n"))
		return response, nil
	}
	// if it is any other error, return a regular error
	if err != nil {
		log.Errorf("error executing transaction: %v", err)
		return nil, err
	}

	// Mark the transaction as successfully committed
	if !dryRun {
		// Mark the transaction as successfully committed
		transactionGuard.Success()

		log.Infof("Transaction: %s - transacted", transactionId)
	}
	return response, err
}

func updateToSdcpbUpdate(lvs tree.LeafVariantSlice) ([]*sdcpb.Update, error) {
	result := make([]*sdcpb.Update, 0, len(lvs))
	for _, lv := range lvs {
		path := lv.GetEntry().SdcpbPath()
		value := lv.Value()
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

	if !d.dmutex.TryLock() {
		return ErrDatastoreLocked
	}
	defer d.dmutex.Unlock()
	// everything remains as is
	return d.transactionManager.Confirm(transactionId)
}

func (d *Datastore) TransactionCancel(ctx context.Context, transactionId string) error {
	log.Infof("Transaction %s - Cancel", transactionId)

	if !d.dmutex.TryLock() {
		return ErrDatastoreLocked
	}
	defer d.dmutex.Unlock()

	return d.transactionManager.Cancel(ctx, transactionId)
}
