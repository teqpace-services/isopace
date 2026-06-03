# Roadmap to v1.0.0

`v1.0.0` is not a quality milestone — it is a **contract**. Per the
[versioning policy](docs/versioning.md), v1 promises a stable public API across
every importable package until v2, and for a payments framework it implies
"safe to run in production." This document tracks what must be true before we
can sign that contract.

> **Status:** the `0.x` API is still moving — `v0.2.0` made a breaking removal
> and `v0.3.0` reintroduced what `v0.2.0` removed (`Zone()`), all within 48h.
> The stability clock effectively started at `v0.3.0` (2026-06-02).

## Ownership legend

- 🛠 **Eng** — code/docs; can be executed in-repo.
- ⚖️ **Counsel** — requires legal review/sign-off; engineering cannot substitute.
- 🔑 **Founder input** — depends on a fact or decision only the team holds.
- 📣 **GTM** — go-to-market / adoption / time.

---

## Definition of Done for v1.0.0

- [ ] **B1 — Production crypto path shipped.** A certified-HSM `Vault`
      implementation exists and is documented as the production default.
- [ ] **B2 — Production integrations delivered**, not just "designed": at least
      OpenTelemetry (`Observer`) and a SQL `Store`, as separate modules.
- [ ] **B3 — Commercial license finalized** by counsel (no "draft / pending").
- [ ] **B4 — jPOS provenance resolved** for every shipped package; clean-room
      claim and dual-license model are defensible.
- [ ] **B5 — Adoption signal**: ≥2–3 real integrations, ≥1 production-like.
- [ ] **API freeze**: a stability soak (≥1 quarter / several minors) with **zero
      breaking changes** to the importable surface, after a deliberate API audit.

---

## B1 — Production crypto path (HSM)  🛠 ⚖️ 🔑

**Problem.** The only shipped backends are software (`SoftVault`, `SealedVault`),
and our own security note says production PIN/key handling *must* use a certified
HSM. Shipping v1 of a payments framework with "not for production" crypto is a
contradiction.

**Constraint.** A PKCS#11 binding pulls in cgo/third-party deps, which would
break the **stdlib-only** core and the clean dual-license. Therefore the HSM
backend must live in a **separate module** (own `go.mod`), implementing the core
`vault.Vault` interface. The core stays stdlib-only; integrators opt in.

**Tasks**
- [ ] 🛠 Confirm the `vault.Vault` interface is sufficient for an external
      backend (PIN block translate, MAC/CMAC, DUKPT, key import/TR-31, ARQC/ARPC).
- [ ] 🛠 Create `adapters/pkcs11/` as a separate module; implement `Vault` over a
      PKCS#11 library.
- [ ] 🛠 Test against **SoftHSM2** in CI (a free PKCS#11 token) for functional
      coverage.
- [ ] 🔑 Identify the target certified HSM(s) (Thales, Futurex, AWS CloudHSM, …)
      for real-hardware validation — **needs hardware/vendor access**.
- [ ] ⚖️ Confirm no certification/compliance claims (PCI, FIPS) are made beyond
      what has been independently validated.

**Exit criterion.** Functional HSM-backed `Vault` passing the conformance suite
against SoftHSM in CI, and at least one real certified HSM validated by hand.

---

## B2 — Production integrations delivered  🛠 🔑

**Problem.** OpenTelemetry, a SQL `Store`, and HSM are described as "drop-in
adapters in separate modules" — but those modules do not exist yet.

**Proposed multi-module layout** (keeps the root module stdlib-only):

```
isopace/                  # root module — stdlib-only (unchanged)
adapters/
  otel/    go.mod         # runtime.Observer -> OpenTelemetry traces/metrics
  sql/     go.mod         # store.Store -> database/sql (+ a driver, integrator-chosen)
  pkcs11/  go.mod         # vault.Vault   -> PKCS#11 HSM   (B1)
  nats/    go.mod         # space.Space   -> NATS/JetStream (optional, later)
```

**Tasks**
- [ ] 🛠 `adapters/otel/` — implement `runtime.Observer` over the OTel SDK;
      example wiring in `examples/`.
- [ ] 🛠 `adapters/sql/` — implement the `store.Store` interface over
      `database/sql`; document driver choice is the integrator's (so no driver
      enters our graph). Migrations + a Postgres integration test.
- [ ] 🛠 CI matrix to build/test each adapter module independently.
- [ ] 🔑 Decide scope for v1: which adapters are **v1-blocking** vs. post-v1
      (recommend OTel + SQL block v1; NATS is post-v1).

**Exit criterion.** OTel and SQL adapters released and documented; root module
graph still shows zero third-party deps.

---

## B3 — Commercial license  ⚖️

**Problem.** `COMMERCIAL-AGREEMENT.md` is a draft "pending counsel review." v1 is
a stronger commercial commitment; the license it is sold under must be final.

**Tasks**
- [ ] 🛠 Produce a counsel-ready package: the draft + a checklist of open
      questions (grant scope, warranty/liability, indemnity for IP provenance —
      see B4, support SLAs, export/sanctions, governing law).
- [ ] ⚖️ Counsel finalizes `COMMERCIAL-AGREEMENT.md` and `COMMERCIAL-LICENSE.md`.
- [ ] ⚖️ Confirm the CLA + dual-license chain is airtight given B4.

**Exit criterion.** Counsel-approved commercial license with no draft markers;
provenance indemnity reconciled with B4.

---

## B4 — jPOS provenance  🔑 ⚖️ 🛠  *(also a present-day risk, not only a v1 gate)*

**Problem.** `README`/`CHANGELOG` state the `CoralPay` and `Zone` profiles were
"generated field-for-field from a certified jPOS `GenericPackager` definition
(`fields2.xml` / `zone.xml`)." jPOS is AGPL-3.0. If those profiles are derived
from jPOS's XML artifacts, they may be derivative works carrying AGPL
obligations — which conflicts with offering them under a commercial license and
with our own clean-room rule ("never copy its code or test fixtures").

**This is gated on a fact only the team holds** (see open question Q1 below):

- **If built from primary sources** (ISO-8583 spec + the network's own published
  field tables; jPOS used only as a black-box wire check):
  - [ ] 🛠 Correct the wording in `README.md` / `CHANGELOG.md` to describe the
        *actual* provenance (remove "generated from jPOS definition" framing).
- **If transcribed/generated from jPOS XML**:
  - [ ] 🔑 ⚖️ Treat as a real provenance issue: re-derive the affected profiles
        from primary sources, **or** obtain licensing clarity. Do **not** merely
        reword.
- [ ] 🛠 Soften combative comparative copy ("How This Beats jPOS" →
      "How Isopace differs from jPOS") and add a trademark/non-affiliation note.

**Exit criterion.** Every shipped profile has documented, defensible provenance;
clean-room claim holds; no AGPL leakage into commercially-licensed packages.

---

## B5 — External adoption  📣

**Problem.** 1 star, 0 forks, no known integrators. The value of an API-freeze
promise is realized *after* real users hit the rough edges. We have not gathered
that signal.

**Tasks**
- [ ] 📣 Identify 2–3 design-partner integrators (ideally existing switch
      operators) to run `0.x` in earnest.
- [ ] 🛠 Lower the on-ramp: the docs site (done), a "Production readiness"
      guide, and a reference deployment.
- [ ] 🛠 Set up an issue triage / feedback loop so integration friction feeds the
      API audit.

**Exit criterion.** ≥2–3 integrations exercising the public API, ≥1
production-like, with their friction folded into the pre-freeze API audit.

---

## Sequencing

1. **Now (cheap, unblocks risk):** B4 wording/provenance decision (Q1), draft
   this roadmap, soften jPOS comparative copy, API-surface audit of the core.
2. **Next (Eng build-out):** B2 OTel + SQL adapters; B1 PKCS#11 adapter on
   SoftHSM.
3. **In parallel (their orgs):** B3 counsel review; B5 design-partner outreach;
   B1 certified-hardware validation.
4. **Gate:** API freeze + stability soak → tag `v1.0.0-rc.1` → integrations →
   `v1.0.0`.

## Interim options (capture momentum without over-promising)

- Tag a **`v1.0.0-rc.1`** to invite integration feedback under a "may still
  change" banner.
- **Narrow the v1 surface**: declare the core (`iso8583`, `packager`,
  `fieldcodec`, `lengthcodec`, `render`) stable while explicitly labeling the
  higher layers experimental until they too have soaked.
