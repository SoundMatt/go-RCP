# TC18 SHOULD/MAY clauses with no corresponding requirement

Companion to `.fusa-reqs.json`'s MUST/SHALL requirement catalog. Mirrors
c-RCP's, cpp-RCP's, and rust-RCP's own identical audits (see those repos'
`docs/TC18-NON-NORMATIVE-CLAUSES.md`). Every SHOULD/MAY occurrence in TC18
is accounted for here or via an existing `REQ-*` entry's new `tc18`
citation field (introduced by this pass — this repo's schema previously
had no citation field).

No spec text is reproduced verbatim; each line is paraphrased and cited
by section/`pdftotext`-extraction line reference.

**Depth note:** same lighter-depth methodology as the cpp-RCP/rust-RCP
passes (12/603 requirements had a `tc18` citation before this pass).
Three MAY clauses below are flagged ⚠ as genuinely uncertain rather than
cited. This pass is independent of, and does not touch, this repo's much
larger known gap — the conditional-request envelope layer
(Compound/CompoundWait/Triggered/Timed all independently invented, not
TC18-conformant — see this project's master checklist and its own
dedicated memory record). A fuller MUST-clause citation backfill remains
separate, larger, future work.

## SHOULD (10, all non-testable — identical disposition across every x-RCP repo)

Six §2 design-goal statements (L773/790/795/798/803/805), two
client-request-composition advice pieces for repetitive compound/
compound-wait requests (L1213/L1312), one client-config-authoring
byte_bus_id/endpoint-type-consistency advice (L2988), one ISELED
hardware-calibration guideline (L5525, moot here — no ISELED endpoint in
this repo). None are library-testable; see c-RCP's own document for the
identical per-line reasoning.

## MAY — implemented and now cited (7)

`REQ-RCS-001` (three lifecycle states, L2063), `REQ-SPI-001` (SPI 6
channels, L4192), `REQ-ADC-004` (samples-per-interval R/W choice,
L5040), `REQ-REQ-004` (fewer sequencers permitted, L3473), `REQ-REQ-006`
(Triggered requests exist as an optional feature, L1413), `REQ-REQ-024`
(Safety_Measure-driven safe state, L2935), `REQ-WAKEUP-003` (power modes,
L2268).

**Caveat**: `REQ-REQ-006`'s Triggered/Timed envelope encode/decode and
`REQ-REQ-004`'s sequencer-register model both live inside
`request/envelope.go`/`request/sequencer.go` — the same files this
repo's much larger, separately-tracked conditional-envelope gap concerns.
The citations above are about the MAY clause's *optionality* (the
capability exists at all), not a claim that the wire format underneath is
TC18-conformant — it is not; see the dedicated gap record.

## MAY — genuinely uncertain / not yet resolved (3, ⚠ needs follow-up)

| § | Citation | Paraphrase | Finding |
|---|---|---|---|
| §12.9.1.1 | TC18.txt L3220 | An RCP frame may include multiple ACF-types (requests) | ⚠ **No single clean citation found**: nothing in `.fusa-reqs.json` was found under a distinctly-titled "multi-request-per-frame dispatch" requirement the way c-RCP's `REQ-MOCK-019` or cpp-RCP's `REQ-L2-007` do. May be covered under a differently-worded existing requirement — needs a closer read, not assumed absent. |
| §13.2 | TC18.txt L3502 | An endpoint may be used or not used in a specific RC Server instantiation (EP_USED bit) | ⚠ **Not found**: no `EpUsed`/`ep_used` concept found. Same finding as cpp-RCP and rust-RCP; c-RCP remains the only repo confirmed to model this bit. |
| §13.7.13.1 | TC18.txt L5631 | An RC Server with an integrated PHY may allow access to it via the MDIO EP | ⚠ **Not addressed**: `mdio`'s functional-config model (`REQ-MDIO-001` through `009`) doesn't discuss this deployment mode the way c-RCP's `ep_mdio.h` does. Likely fine by the same reasoning but not independently confirmed. |

## MAY — descriptive/out-of-scope (27, identical disposition to the other three x-RCP repos' own audits)

L640 (Edge Node PTP/MACsec — L1/L2 topology, out of RCP-wire scope),
L909, L1024, L1025, L1588, L1943 (gPTP sync generally — no single
citable requirement), L2060, L2062, L2244, L2289, L2355, L2385, L2405,
L2565, L2668, L2984, L2986, L2989, L3197, L3206, L3227, L3252, L4323,
L5035, L5164, L5358 — each is descriptive prose restating an
architectural fact from a different angle, a client-side/hardware-
deployment choice outside library scope, or a non-closed-list permission.
See c-RCP's identical document for the per-line paraphrase and
reasoning — the spec text and its non-normative character don't change
per implementation.
