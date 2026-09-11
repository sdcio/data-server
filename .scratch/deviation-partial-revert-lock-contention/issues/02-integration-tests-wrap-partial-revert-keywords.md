# 02 — integration-tests: wrap the two partial-revert keywords in retry

**Status:** done

**What was built:** completed the fix started by `integration-tests` commit
`c69c07e` ("fix(deviations): retry Delete Deviation on transient
datastore-locked errors"). That commit's own closing ticket
(`.scratch/last-applied-snapshot-write-at-apply/issues/08`) named two
originally-failing groups — "Reject Deviations" and "Partially Revert
Deviations by Filter Path" — but only wrapped the keyword behind the first
(`Delete Deviation`). This issue wraps the second group's two keywords, one
per device family, in the same `Wait Until Keyword Succeeds
${eventual_timeout} ${retry}` pattern, each extracting a `Run Partial Revert
Deviations by <X>` helper (mirroring `c69c07e`'s `Run Deviation Revert`
extraction) so the RPC call itself — not the whole keyword including its
argument-resolution steps — is what gets retried:

- `Partial Revert Deviations For Intent by Interface`
  (`tests/03-deviations/22-srl-nonrevertive.robot`) → extracts
  `Run Partial Revert Deviations by Interface`.
- `Partial Revert Deviations For Intent by Admin State`
  (`tests/03-deviations/21-sros-nonrevertive.robot`) → extracts
  `Run Partial Revert Deviations by Admin State`.

**Blocked by:** none.

**Verification:** `robot --dryrun` on both files parses and resolves cleanly
(20/20 and 16/16 tests, no errors).

## Comments

Repo: `integration-tests`, branch `config-server-cache-backend`. This is a
CI-level safety net independent of `config-server` issue 01 (the actual
protocol-level fix) — defense in depth, not a substitute for it.
