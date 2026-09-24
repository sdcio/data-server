# 04 — config-server: `deviceProfile` on data-server base stack

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** config-server (successor to #484) wires `deviceProfile` through the KRM layer against the **data-server device-profile base** branch, using **both** proto enum values in CRD/spec. Creating a Target with `sonic` or `cisco-ios-xr` fails with the same **not enabled** semantics as data-server until the matching NOS data-server PR merges. Protos revision aligns with the data-server base `go.mod` pin (commit on sdc-protos#120), not a separate protos-main release step.

**Blocked by:** 03 — Device-profile base PR

**Status:** ready-for-agent

- [ ] Schema and wiring expose generic, `sonic`, and `cisco-ios-xr` consistently with protos
- [ ] CreateDataStore (or equivalent) does not advertise end-to-end SONiC/Cisco success before the corresponding data-server NOS PR lands
- [ ] Paired-test / integration docs call out preconditions when E2E implies working NOS on data-server
