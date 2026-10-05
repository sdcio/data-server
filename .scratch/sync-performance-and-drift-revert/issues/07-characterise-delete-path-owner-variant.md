# 07 — Characterise delete-path owner-variant behaviour

**What to build:** A test-only prefactor that pins down today's behaviour when a delete-path intent's owner already has a real variant at the same leaf (the existing variant is converted in place to an explicit delete), plus the other precedence outcomes of delete-path intents (wins over Running and defaults, loses to real intent values). This makes the lazy-coverage rewrite safe.

**Blocked by:** None — can start immediately.

**Status:** done

- [x] Test covers owner-has-real-variant-at-same-leaf and records the current outcome
- [x] Tests cover delete-path vs Running, defaults, and other intents' real values
- [x] Tests pass on current code without production changes
