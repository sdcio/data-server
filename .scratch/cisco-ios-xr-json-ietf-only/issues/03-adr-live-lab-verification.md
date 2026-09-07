# 03 — New ADR: live-lab verification + encoding-restriction decision

**What to build:** A new ADR in `docs/adr/` (next available number) documenting two things: (a) that ADR 0001's module-anchored `permodule` mechanism was verified against real Cisco XRd hardware and found correct as-is, with the specific probes and results; (b) the `JSON_IETF`-only restriction (tickets 01/02) as a distinct decision, with the categorical-rejection evidence for `PROTO`/`JSON`.

**Blocked by:** 01, 02 — write this once the actual code/test changes exist, so the ADR describes what shipped rather than what was planned.

**Status:** ready-for-agent

- [ ] New file `docs/adr/000N-cisco-ios-xr-json-ietf-only-live-lab-verified.md` (or similar slug; number picked at write time from the next available slot in `docs/adr/`).
- [ ] Content per spec.md's "ADR" implementation decision: what was tested (local `containerlab cisco_c8000`/8201-32FH XR 7.10.1 instance, `clab-cisco-ixr01`), against which shapes (per-leaf vs. module-root, `origin: <module>` unprefixed vs. `origin: cisco_native` prefixed vs. no-origin prefixed, plain `json_val` vs. `json_ietf_val`, `PROTO` scalars, multi-module single-`SetRequest`, module-root delete, delete+update replace), and the pass/fail result for each (see spec.md's Further Notes for the full list).
- [ ] Explicitly state that ADR 0001 is **not** superseded — its mechanism was confirmed, not contradicted — and that this ADR only adds the `JSON_IETF`-only restriction as a separate decision plus the verification record.
- [ ] Explicitly flag the version gap: verified on XR 7.10.1, issue #483's reporter is on XR 26.2.1 — noted as residual risk, not fully closed by this verification.
- [ ] Follow the "Why" / "Considered options" ADR shape already used by `docs/adr/0001-ios-xr-granular-gnmi-json-from-api-entry.md` and `0002-southbound-set-materialization-and-targetsource-retirement.md`.

## Comments
