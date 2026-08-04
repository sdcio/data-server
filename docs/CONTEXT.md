# SDC Data Server

One **Datastore** per managed **Target**: prioritized **Intents** merge with **Running** (device) state, validate against **Schema**, and change through a **Transaction** (confirmed-commit) lifecycle. **Platform and control-plane owners** use this file for shared vocabulary; **implementers** use [architecture.md](./architecture.md) for the high-level system picture and [development.md](./development.md) for conventions—RPC fields, enums, and step-by-step behavior live in **protos and code**, not in architecture.md.

## Language

**Datastore**:

A logical per-target unit that binds YANG **Schema**, holds merged configuration state, talks to one **Target** (southbound), and participates in **Transactions** and **Sync**.

_Avoid_: Using “datastore” to mean only the cache or only the gRPC service — here it is the full per-target runtime object.

**Intent**:

A named, prioritized configuration contribution stored as a blob and merged with other intents under precedence rules; distinct from **Running**.

_Avoid_: “Config file” for a persisted intent unless the audience is purely storage-focused.

_Avoid_: **`device-profile`** (or other **SBI** / **Target** wire options) as an **Intent** field — those belong to **Datastore** configuration, not intent storage.

**Non-revertive intent**:

An **Intent** whose values are written to the **Target** on first appearance but whose divergence from **Running** is **not** automatically corrected afterwards. If a non-revertive-owned leaf later disappears from the device (out-of-band CLI edit, Ansible push, firmware reload, etc.) the datastore records a **Deviation** (`NOT_APPLIED`) and surfaces it to subscribers, but does **not** re-push the value. The **Intent blob is still the expected state** — it has not been retracted — so the gap is visible and auditable until an operator resolves it. First-appearance write semantics are identical to a normal (revertive) intent; the only difference is that converge-on-drift is disabled for that owner.

_Avoid_: Calling non-revertive intents "read-only" or "advisory" — they are still applied once and still authoritative for blame and deviation reporting; they just do not trigger automatic reapply when the device diverges.

**Owner**:

The **intent name** that owns a merged value for precedence, export, and blame (the same identity as **Intent** in APIs and storage); **`"running"`** is reserved for **Running**.

_Avoid_: Treating **Owner** as a separate principal from **Intent** — here it is always “which intent name owns this leaf,” not a second identifier.

**Running**:

The live **Target**’s configuration as reflected in the datastore (via **Sync**), merged under owner **`"running"`** like other layers but not authored as a northbound **Intent**; **Deviation** and blame treat it differently from ordinary intents in important cases.

_Avoid_: Confusing **Running** with a normal user intent of the same name — **`"running"`** is device truth materialization for this component, even though it is stored in a shape compatible with **Intent** blobs for export and comparison.

**Transaction**:

A confirmed-commit unit of work: **TransactionSet** (validate, apply, persist, arm rollback), then **TransactionConfirm** or **TransactionCancel** / timeout (rollback to prior intents and device state).

_Avoid_: “Transaction” meaning only a database transaction or only an RPC id without the two-phase semantics.

**Deviation**:

A per-path signal comparing merged **Intent** ownership and values to **`"running"`** for subscribers (for example config-server), exposed on the wire as protobuf **`DeviationReason`** (`WatchDeviationResponse.reason` in `data.proto`). When you say “drift,” name **`NOT_APPLIED`**, **`UNHANDLED`**, **`OVERRULED`**, **`INTENT_EXISTS`**, or **`DR_UNKNOWN`**; exact numeric wire values and protobuf comments are **authoritative** in the `data.proto` definition (generated Go mirrors the same enum). **Non-revertive intents** produce `NOT_APPLIED` deviations when the device diverges without triggering a reapply — see **Non-revertive intent** above.

_Avoid_: “Drift” without naming whether you mean intent vs **Running**, device-only configuration, or a losing **Owner** among intents.

**Confirmed applied value**:

The value observed on the **Target** via a targeted read-back immediately after a **Datastore**'s own successful write for one `(Owner, path)`, stored separately from both the **Intent** value and **Running**. For a **mutate-on-write leaf**, **Deviation** comparison uses this value as the drift baseline instead of the owning **Intent**'s literal value — so a device-side mutation (for example a device hashing a pushed secret) is captured once as "expected" and does not itself produce a **Deviation**. If no confirmed applied value exists yet for the current highest-precedence **Owner** (nothing has been successfully written by it yet), comparison falls back to the ordinary **Intent**-value comparison used for every other leaf.

_Avoid_: Treating this as a substitute for **Running** — **Running** is still the live device-truth materialization for every leaf; the confirmed applied value is an additional, narrower baseline that only matters for **mutate-on-write leaves**, and only for the currently-winning **Owner**.

**Mutate-on-write leaf**:

A **Schema** leaf or leaf-list where a successful write can be mutated by the **Target** before the next read (for example a device hashing a pushed secret), so the value **Running** reflects afterward is expected to differ permanently from the value the **Intent** sent. Declared by either of two independent, unioned sources — mirroring the **Sensitive leaf** dual-source model: (1) a **schema-level** YANG extension (parallel to `sdcio-ext:sensitive`, see [ADR 0003](./adr/0003-yang-extension-schema-sensitive-baseline.md)), applying to all instances of that leaf; (2) an **intent-level** per-path marker submitted at **Transaction** time (parallel to `sensitive_paths`, see [PR #460](https://github.com/sdcio/data-server/pull/460)). Either source marking a path is enough — the union always wins, matching the cross-intent union direction used for **Sensitive leaf** in merged views ([ADR 0004](./adr/0004-scoped-sensitive-path-union-per-operation.md)), since **Deviation** computation is itself a merged view.

_Avoid_: Confusing this with **Non-revertive intent** — non-revertive is a per-**Intent**, per-path opt-out of auto-correction that still reports drift honestly; **mutate-on-write leaf** changes what counts as drift in the first place, for a specific, narrow, schema-identified reason, and still reports genuine external drift (a value diverging from the confirmed applied value) normally.

_Avoid_: Introducing a general-purpose "ignore this deviation" escape hatch. Explicitly rejected — a generic suppress-reporting flag would hide real drift, which **Non-revertive intent** and **mutate-on-write leaf** both deliberately do not do.

**Schema**:

The bound YANG context (vendor, name, version) used to expand paths, validate values, and interpret the configuration tree for a **Datastore**.

_Avoid_: “Schema” for ad-hoc JSON shapes unrelated to that YANG binding.

**Standards (normative specifications)**:

Where published specs define observable behavior for a path this component implements (for example IETF YANG/NETCONF where used, **gNMI** southbound where used), those documents are the default **normative** interoperability contract; gaps stay **narrow, conscious, and documented** in maintainer docs (for example ADR-style notes in [architecture.md](./architecture.md)), focused PRDs, and tests—not informal habit.

_Avoid_: Treating applicable standards as ignorable for wire-visible behavior when nothing explains a real-world deviation.

**Union validation (ambiguous union members)**:

When an encoding does not preserve which **union** member applies, validation follows documented rules so valid values are not rejected without cause; **scalar** vs **leaf-list** behavior and integrator policy live in [UNION-INGRESS.md](./prd/union-member-resolution-validation/UNION-INGRESS.md) (high-level stance: [architecture.md](./architecture.md#7-southbound-and-interoperability)).

_Avoid_: Saying “validated” without clarifying full member-specific rules vs best-effort when member metadata is missing.

**Target**:

The managed network element reached southbound (for example gNMI, NETCONF, or the explicit mock kind **`noop`**); each **Datastore** owns exactly one live **Target** handle—wire encoding, optional **`device-profile`**, and how replace reaches the device are southbound concerns ([architecture.md](./architecture.md#7-southbound-and-interoperability)), not the northbound **Intent** model. Profile-specific write shaping (for example granular gNMI JSON for IOS-XR) should follow the **schema-interpreted merged configuration tree**, not ad hoc structure inferred solely from a monolithic serialized JSON blob.

_Avoid_: “Target” for unrelated deployment targets (Kubernetes nodes, build targets).

**noop (southbound)**:

The explicit mock **Target** / **SBI** kind whose purpose is to exercise the full data-server pipeline — tree building, **Schema** validation, **Intent** merging, deviation tracking, and **Transaction** lifecycle — without requiring a real network device. The pipeline runs end-to-end up to and including the apply step; the apply itself is a no-op (synthetic response, nothing sent on the wire). **Running** stays empty because there is no device to sync from. Any connection-profile fields (address, port, TLS, credentials, **Sync** config) are silently ignored. Callers MUST set southbound **`type`** explicitly to **`noop`**; an empty **`type`** is invalid.

_Avoid_: Inferring **`noop`** from a missing southbound **`type`**.

_Avoid_: Treating **`noop`** as "disabled" or "offline" — it is an intentional full-pipeline exerciser, not a degraded mode.

**Southbound set materialization**:

Deriving the concrete apply operations for a **Target** from merged, schema-validated configuration together with that **Target**’s southbound settings (protocol, encoding, optional **`device-profile`**, and transaction semantics such as merge vs replace where they change wire shape). This is the policy and encoding step **before** sending those operations on the wire; it is distinct from transport-only handling of an already-formed operation set.

Maintainer rationale for the **Set**-only boundary (**typed plans**, **`TargetSource`** retirement on **Set**, **`Get`** still deferred): [ADR 0002](./adr/0002-southbound-set-materialization-and-targetsource-retirement.md).

_Avoid_: Using “materialization” for **Running** / **Sync** (device → datastore) without saying so—or using it interchangeably with northbound export of **Intents**.

**Southbound apply input**:

The merged, **Schema**-interpreted configuration state plus explicit apply semantics (for example merge vs replace where they change wire shape) supplied to **Southbound set materialization** for one apply step. It is **not** a single northbound **Intent** blob—**Intents** merge with each other and with **Running** under precedence rules before this input exists.

Concrete server-side type: `SouthboundApplyInput` in `pkg/datastore/target/materialize`.

_Avoid_: Calling this an “**Intent** snapshot” when you mean merged datastore state—that collides with **Intent** as a named northbound contribution.

**Southbound set plan**:

The discriminated payload passed to **`Target.Set`** after materialization (`SouthboundSetPlan` in `pkg/datastore/target/types`): either a **gNMI** plan (`GnmiSetPlan` in **materialize**—**Updates** and/or **Replaces** plus **Deletes**, depending on profile and merge vs replace) or a **NETCONF** XML document, depending on datastore **SBI** type. The datastore builds it in one place (`buildSouthboundSetPlan`); transports apply it without re-running encoders.

**Granular gNMI configuration (IOS-XR device profile):**

Southbound expression of merged YANG-backed state as one or more **gNMI** **`Update`** and/or **`Replace`** messages (module-anchored paths, list keys from **Schema**, JSON vs JSON_IETF as a value encoding choice). The **canonical** source of path and list structure is the merged **schema-attached tree** as **`api.Entry`**, not structure inferred only from an untyped JSON object. Encoding may start from the datastore **root** or from any **schema-consistent subtree**; emitted paths follow **root-based** gNMI conventions. Merge vs replace **intent** is an explicit caller input at this boundary, not something the encoder deduces from payload shape.

**`origin` field** in gNMI paths for IOS-XR native YANG is the **full YANG module name** (for example `Cisco-IOS-XR-ip-static-cfg`); all **`Update`** messages for a given module share the same `origin` value and are grouped within a **single `SetRequest`** (not split across multiple RPCs). `origin` annotation is an **encoder concern**: it must be applied in the IOS-XR-specific encoder, not in the shared tree or path-production layer, because other devices (for example SR Linux) only accept their own fixed `origin` tokens (`openconfig`, `native`) and would reject unknown module names.

_Avoid_: Long-term reliance on parallel caller hints (for example ad-hoc “list split” rules) for information already carried by **Schema** and **Entry** when that tree is the input.

_Avoid_: Setting `origin` on `sdcpb.Path` at the tree / ops layer — `ToGNMIPath` passes it straight to wire and would send IOS-XR module names to every gNMI target.

**Sync**:

Mechanisms that update **Running** in the datastore from the live device (subscriptions, polling, or one-shot reads) according to datastore sync configuration.

**Running refresh (southbound read path)**:

The **Sync** side of the house: observing live device configuration for configured paths and updating **Running** in the datastore. It is the inverse direction of **Transaction** apply (merged state → device); both may touch **Running**, but **Running refresh** is always “device truth in,” not “intended config out.”

Two common **southbound read patterns** both count as **Running refresh**: a **snapshot-led** read (a bounded answer at a point in time, for example polling `Get` or `get-config`) and an **update-led** read (time-ordered changes from a subscription or similar). The northbound model is the same; only how device truth is obtained differs.

_Avoid_: “Sync” for northbound cache replication unless that subsystem is explicitly in scope.

_Avoid_: Conflating **Running refresh** with **Southbound set materialization**—the latter shapes how merged state is applied **to** the **Target**; the former shapes how device state is read **for** **Running**.

**Northbound**:

Callers of the data-server gRPC API (for example config-server or tooling) that create datastores, submit transactions, and query intents or deviations; datastore **SBI** fields (including **`device-profile`** when present) round-trip in northbound datastore config, not inside ordinary **Intent** payloads.

_Avoid_: “API” without saying northbound versus southbound.

**Sensitive leaf**:

A YANG leaf or leaf-list whose northbound-rendered value is replaced with the fixed sentinel `***` unless the caller sets `include_sensitive: true`. Two independent sources of sensitivity compose at render time: (1) **schema-level flag** — leaf annotated with `sdcio-ext:sensitive` in a device-profile YANG overlay, applies globally to all instances; (2) **intent path markers** — `sensitive_paths` stored per-**Intent** in cache, scoped by operation: for `GetIntent(regular)` only the fetched intent’s own markers apply; for `GetIntent(running)`, `BlameConfig`, and `WatchDeviations` the union of all non-running intents’ markers applies. Sensitivity is a property of the **schema path**, not a list instance — list-key predicates are stripped. The southbound path is unaffected.

_Avoid_: Confusing "sensitive" with encrypted-at-rest (a separate, unimplemented concern); the flag controls northbound visibility only.

**Schema key order**:

The order list key leaf names appear in the bound YANG `key` statement (for example `key "key2 key1"` → `key2` before `key1`). NETCONF and other southbound encodings that care about key element order must follow this order (RFC 7950 §7.8.5).

_Avoid_: Assuming schema key order matches alphabetical order — they often differ.

**Tree key level order**:

The order key-level nodes are organized under a list entry in the merged **schema-attached tree**: alphabetically by key leaf name, independent of **Schema key order**. Code that walks up or down key levels must pair each level with the correct key name using this order.

_Avoid_: Using `levelsUp` as a direct index into unsorted schema keys — that pairs the wrong key name when schema and tree order differ.

## Relationships

- One **Datastore** binds one **Schema** tuple, one **Target**, and many **Intents** plus the system **Running** materialization.
- **Intents** (northbound) merge by priority; each contributed value carries an **Owner** (intent name). **Running** is the sole southbound-sourced “intent-shaped” layer (`"running"`): **Sync** keeps it aligned with the live **Target** while **Transactions** update it after successful apply.
- A **Transaction** validates and applies the merged result to the **Target**, updates intent blobs, and updates **Running**; confirm commits, cancel or timeout rolls back.
- **Deviations** summarize per-path disagreement for subscribers using **`DeviationReason`**; see **Deviation** above and the protobuf enum definition in `data.proto`.

## Example dialogue

> **Dev:** “If we **Confirm** late, is the device already in the new state?”  
> **Domain expert:** “Yes — **TransactionSet** already applied to the **Target**. **Confirm** only stops the rollback timer and drops bookkeeping; **Cancel** or timeout replays prior **Intents** and device state.”

> **Dev:** “Is **Running** just another **Intent**?”  
> **Domain expert:** “It is merged under an **Owner** like other layers and stored in an intent-compatible way for export and blame, but it is the southbound reflection of device truth — not something you treat as a normal northbound **Intent** in every calculation.”

## Flagged ambiguities

- **Empty southbound `type`**: Must not mean **`noop`**. **`noop`** is only valid when **`type`** is explicitly **`noop`**; missing or empty **`type`** must be treated as invalid input at every layer — `target.New` must return an error, not silently fall through to noop.
- **NETCONF “candidate” datastore** (RFC 6241) is a protocol concept on the device. It is **not** the historical SDC “candidate datastore” type; that older product concept has been removed from the current data-server design. When docs or CLI examples mention “candidate” in an SDC sense, treat them as **stale** until updated to the current model.
