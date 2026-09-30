# Legacy commit inventory — PR #442 / PR #480 → parallel stacks

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)  
**Spec:** [10-device-profile-parallel-stacks-spec.md](../open/10-device-profile-parallel-stacks-spec.md)

**Merge-base with `origin/main` (inventory anchor):** `f24dd6c48e1905139408d78edd7ce94512e36113`

| Legacy PR | Branch | Commits (oldest → newest on stack) |
|-----------|--------|-------------------------------------|
| [#442](https://github.com/sdcio/data-server/pull/442) | `ciscoiosxrd2` | 8 commits above merge-base |
| [#480](https://github.com/sdcio/data-server/pull/480) | `sonic-device-profile` | 37 commits above merge-base (includes all #442 commits) |

**Buckets:** `base` = device-profile base PR · `main` = generic fix to `main` · `sonic` = SONiC NOS PR · `cisco` = Cisco IOS-XR NOS PR (draft) · `meta` = tracker/scratch only (do not port)

---

## Protos dependency (agreed pin)

Merge **[sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120)** and pin on the **device-profile base** branch — not a separate delivery ticket.

| Item | Value |
|------|--------|
| Branch head (both enum values) | `40ed0bc26a71263302a5b24b741ab67db4fac01f` |
| Pseudo-version (as on `sonic-device-profile`) | `v0.0.55-0.20260828064538-40ed0bc26a71` |
| `go.mod` pattern | `replace github.com/sdcio/sdc-protos v0.0.55 => github.com/sdcio/sdc-protos v0.0.55-0.20260828064538-40ed0bc26a71` until a tagged release includes #120 |

**Note:** `ciscoiosxrd2` still pins an older proto commit (`67240812f373`, IOS-XR enum only). Rebasing stacks must move to the #120 pin on **base**, not the #442 pin.

---

## PR #442 (`ciscoiosxrd2`) — commit mapping

| SHA | Subject | Bucket | Notes |
|-----|---------|--------|-------|
| `df81f6f` | feat: Cisco IOS-XR gNMI support via module-anchored SetRequest encoding | **base** (+ **cisco** split) | **Split at stack cut:** land shared materialization (`materialize/`, `types/setplan`, TargetSource retirement, thin gNMI/NETCONF/noop drivers, generic `BuildPlan` paths, config field + closed-set validation stubs, server/proto mapping for known profiles) on **base**. Land `gnmi/permodule/*`, Cisco-enabled `BuildPlan` arm, and Cisco-specific config acceptance on **cisco** (see rows below that were extracted later). |
| `38934f7` | Clarify DeviceProfile comment in datastore.go | **base** | Doc comment on shared config type. |
| `4298333` | fix(materialize): skip gNMI Set when JSON tree has no new/updated leaves | **base** | Generic gNMI materialize no-op; not NOS-specific. |
| `b5bc3e9` | config: reject cisco-ios-xr gnmi SBI unless encoding is JSON_IETF | **cisco** | Profile enablement validation (JSON_IETF-only). On **base**, `cisco-ios-xr` remains **not enabled** instead. |
| `3d94d84` | materialize: narrow cisco-ios-xr BuildPlan dispatch to JSON_IETF only | **cisco** | Cisco materialize arm behavior when profile is enabled. |
| `6e93048` | docs(adr): land ADR 0001 (IOS-XR granular gNMI JSON from api.Entry) | **cisco** | IOS-XR encoder narrative; optional short pointer on base ADR index only if needed for reviewers. |
| `0a3d1f5` | docs(adr): add ADR 0002 for cisco-ios-xr live-lab verification + JSON_IETF-only | **cisco** | Live-lab / encoding decision for Cisco profile. |
| `7225650` | docs: mark ticket 04 done, post PR #442 follow-up | **meta** | `.scratch` only; no port. |

---

## PR #480 (`sonic-device-profile`) — commits not in #442

Listed newest-first as on branch; each appears once.

| SHA | Subject | Bucket | Notes |
|-----|---------|--------|-------|
| `1417054` | fix(tree): correct YANG parent navigation from key-value nodes in NavigateSdcpbPath | **main** | XPath/tree navigation; prerequisite for SONiC must-statements / integration. |
| `cbb179d` | fix(utils): scope RFC7951 self-wrap unwrap to keys that don't resolve as real schema members | **main** | Generic converter; follow-up to unwrap in `95b8408`. |
| `b3aecc4` | fix(test): set sbiMutex in TestTransactionSet_PreviouslyApplied's manual Datastore literal | **main** | Test fix for `c368539`; land with or immediately after datastore mutex fix. |
| `facd005` | fix(config): drop stale cisco-ios-xr+json-accepted assertion after ciscoiosxrd2 rebase | **meta** | Test hygiene after stack rebase; re-derive assertions when splitting **cisco** / **base** tests. |
| `57f1c7d` | fix(gnmi): wire targetName into Once/Stream subscribe options; gate debug logging in transaction_rpc | **main** | Subscribe `Target` naming + debug gating (spec: subscribe target naming). |
| `95b8408` | fix(sonic-device-profile): normalize nil GnmiSetPlan, unwrap RFC7951 …, improve must-statement error context | **main** (+ **base** split) | **Split:** `targettypes`/nil `GnmiSetPlan` normalization → **base** (shared plan types). `converter` RFC7951 unwrap + `validation_entry_must` context → **main**. |
| `4ea1b4f` | Fix count() crash on leaf-list must-statements for unset leaf-lists | **main** | yang-parser bump + tree adapter; SONiC-motivated, generic behavior. |
| `4275945` | fix(tree): stop evaluating list must-statements at the key-level node | **main** | Generic must validation. |
| `e16aaa7` | fix(tree): avoid sorting shared schema key slice in importConfigTask | **main** | Generic tree import concurrency/correctness. |
| `a2cd8c6` | refactor(gnmi): move sonic Get-shaping behind a device-profile seam | **sonic** | Get ALL / translib shaping at sanctioned seam; **base** should retain seam + stub error until **sonic** enables. |
| `c368539` | fix(datastore): guard sbi target field with mutex to fix data race | **main** | Datastore concurrency (independent of profiles). |
| `cebe70c` | drop .scratch | **meta** | Removes local scratch from branch; do not replicate on product branches. |
| `9e07853` | docs(sonic-device-profile): note the correct worktree/branch to work in | **meta** | Scratch tracker only. |
| `6ddcdcf` | feat(server): expose DeviceProfileSonic on gRPC/Target-CR layer | **base** | Proto bump to #120 pin + **both** enum mappings on gRPC layer; disabled profiles must still error like config validation on **base**. |
| `fa9fa5b` | docs(sonic-device-profile): update spec and tickets to reflect predicate removal refactor | **meta** | Scratch + unrelated cache-client scratch; no port. |
| `4803719` | fix(test): replace bare "gnmi" literals with config.SBITypeGnmi in materialize tests | **base** | Test cleanup tied to exported SBI constants. |
| `4534607` | refactor(config): remove IsSonic/IsCiscoIOSXR middle-man predicates | **base** | Shared config/materialize dispatch style (no tight coupling). |
| `87d9bc8` | refactor(config): export SBI type constants; remove IsGnmi() middle man | **base** | Exported `SBIType*` constants for dispatch guards. |
| `61061d9` | refactor(materialize): remove primitive obsession and duplicated error-wrap | **base** | Shared materialize structure. |
| `69c5760` | chore(sonic-device-profile): mark ticket 04 done | **meta** | Scratch only. |
| `92a3d91` | feat(materialize): wire sonic dispatch into BuildPlan | **sonic** | SONiC materialize arm + tests. |
| `4a1e130` | fix(sonic): commit package rename from parentbound to sonic | **sonic** | Package rename follow-up. |
| `1657f08` | chore(sonic-device-profile): note package rename in ticket 03 | **meta** | Scratch only. |
| `fbfb103` | Rename parentbound encoder package to sonic. | **sonic** | Encoder package rename. |
| `6aad452` | Fix multi-key list wrap-key resolution in parentbound encoder. | **sonic** | Encoder fix. |
| `54f29a3` | chore(sonic-device-profile): note ticket 03 commit in issue tracker | **meta** | Scratch only. |
| `6c7a684` | Add parentbound encoder for SONiC translib Set plans. | **sonic** | SONiC Set encoder. |
| `093c4b2` | chore(sonic-device-profile): note sdc-protos branch pushed | **meta** | Scratch only. |
| `5bd4d84` | chore(sonic-device-profile): cross-ref tickets 02 and 05 | **meta** | Scratch only. |
| `dae3333` | chore(sonic-device-profile): mark ticket 02 done | **meta** | Scratch only. |
| `6de23fa` | feat(config): add sonic device-profile with JSON_IETF-only validation | **sonic** | SONiC profile validation when enabled; on **base**, `sonic` is **not enabled**. Scratch spec under `.scratch/sonic-device-profile/` is **meta** (do not port). |

---

## Cherry-pick bridges (short-lived only)

Prefer **merge to `main` then rebase** stacks. Cherry-pick onto `device-profile-base` only when the base PR cannot build or be reviewed without a fix that is not yet on `main`.

| Fix (bucket **main**) | Bridge when | Precondition link from |
|------------------------|-------------|-------------------------|
| `4ea1b4f`, `4275945`, `1417054` | SONiC integration tests fail on base without tree/xpath fixes | **sonic** PR body |
| `c368539` + `b3aecc4` | Race detector / `applyIntent` tests fail | **sonic** or **base** if base runs those tests |
| `57f1c7d` | Subscribe tests or gnmi build break before `main` merge | **sonic** PR body |
| `95b8408` (converter/must parts) | SONiC lab scenarios hit must/GET unwrap bugs | **sonic** PR body |
| `cbb179d` | After `95b8408` unwrap lands; blocks SRL-style integration | **sonic** PR body + optional **main** PR |

Document any actual cherry-pick SHAs in the **base** PR description when used; remove after rebase onto `main`.

---

## Precondition links (ticket 02 ↔ 05)

Open **main** PRs for rows marked **main** above before merging **sonic** if lab/CI still depends on them. Suggested grouping:

| Suggested `main` PR theme | Commits |
|---------------------------|---------|
| Tree must / XPath navigation | `4275945`, `4ea1b4f`, `1417054`, `e16aaa7` |
| Converter GET parsing | `95b8408` (unwrap + tests), `cbb179d` |
| Datastore concurrency | `c368539`, `b3aecc4` |
| gNMI subscribe target naming | `57f1c7d` |

**Cisco draft** generally depends on **base** + its own **cisco** rows; tree/converter **main** fixes improve Cisco lab work but are not stack-blockers unless IOS-XR tests hit the same paths.

---

## `df81f6f` file-level split (reference)

When cutting **base** vs **cisco** from the monolith:

| Land on **base** | Land on **cisco** |
|------------------|-------------------|
| `pkg/datastore/target/materialize/` (generic paths + stub arms failing closed for both NOS profiles) | `pkg/datastore/target/gnmi/permodule/` |
| `pkg/datastore/target/types/setplan.go` | Cisco-enabled branches in `materialize.go` (later refined by `3d94d84`) |
| Target interface + noop/netconf/gnmi transport consuming plans | Cisco JSON_IETF validation (`b5bc3e9`) |
| Retire TargetSource adapters | ADRs 0001/0002 (`6e93048`, `0a3d1f5`) |
| Config: recognize profiles + reject non-generic on base | |
| Server: enum mapping for all known values + enablement errors | |
| Shared tests for generic materialize + disabled profiles | Cisco encoder + Cisco routing tests |

---

## Checklist (ticket 01)

- [x] Each meaningful commit classified (product commits → single bucket; scratch/chore → **meta**)
- [x] Generic vs NOS vs base boundaries aligned with [parallel-stacks spec](../open/10-device-profile-parallel-stacks-spec.md)
- [x] Mapping checked in at this path for rebase/split work
- [x] Protos recorded as **sdc-protos#120** @ `40ed0bc26a71263302a5b24b741ab67db4fac01f` (pseudo-version above)
