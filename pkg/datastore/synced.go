package datastore

// ensureSyncedInit lazily seeds the per-sync-name Synced tracking state from
// the datastore's configured Sync.Config entries. Zero configured syncs
// (including a nil Sync config, and a noop target discarding a non-empty
// Sync.Config via MarkSynced) immediately latches Synced.
//
// Safe to call repeatedly and from concurrent goroutines; only the first
// call performs the seeding.
func (d *Datastore) ensureSyncedInit() {
	d.syncedMu.Lock()
	defer d.syncedMu.Unlock()
	if d.syncedNames != nil {
		return
	}
	d.syncedNames = make(map[string]bool)
	if d.config != nil && d.config.Sync != nil {
		for _, sp := range d.config.Sync.Config {
			d.syncedNames[sp.Name] = false
		}
	}
	if len(d.syncedNames) == 0 {
		d.synced.Store(true)
	}
}

// MarkSynced records that the named sync mechanism has completed its first
// successful cycle writing to Running. Idempotent: once every configured
// sync name has been marked, Synced latches permanently and further calls
// (including for names not part of the configured set) are no-ops.
//
// This latch is never reset, so it survives target reconnects: AddSyncs is
// only called once at target construction, and reconnect paths do not
// recreate sync objects or call AddSyncs again.
func (d *Datastore) MarkSynced(name string) {
	if d.synced.Load() {
		return
	}
	d.ensureSyncedInit()

	d.syncedMu.Lock()
	defer d.syncedMu.Unlock()

	if d.synced.Load() {
		return
	}

	d.syncedNames[name] = true

	for _, done := range d.syncedNames {
		if !done {
			return
		}
	}
	d.synced.Store(true)
}

// Synced reports whether Running has completed its first successful sync
// cycle from the target, across every configured sync mechanism. Latches
// permanently true once reached and never reverts.
func (d *Datastore) Synced() bool {
	if d.synced.Load() {
		return true
	}
	d.ensureSyncedInit()
	return d.synced.Load()
}

// checkSynced returns ErrNotSynced if Running has not yet completed its
// initial sync from the target, nil otherwise. Shared by TransactionSet's
// replace-phase and merge-phase gates.
func (d *Datastore) checkSynced() error {
	if !d.Synced() {
		return ErrNotSynced
	}
	return nil
}
