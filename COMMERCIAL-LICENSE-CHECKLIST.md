<!--
SPDX-License-Identifier: LicenseRef-Teqpace-Commercial
Copyright (C) 2026 Teqpace Services Ltd. All rights reserved.
-->

# Commercial license — counsel review checklist

> **For legal counsel. Not legal advice; prepared by the engineering team to
> accelerate review.** This checklist turns the draft
> [`COMMERCIAL-AGREEMENT.md`](COMMERCIAL-AGREEMENT.md) into an actionable list of
> decisions and placeholders to finalize before the commercial license is
> offered to, or executed with, any customer. It is a **B3 blocker for
> [v1.0.0](ROADMAP-to-v1.md)**: the license must be final (no "draft / pending")
> before we make the stronger commercial commitment that v1 implies.

The draft agreement is already structurally complete (grant, restrictions, fees,
support, IP, warranties, indemnity, liability, confidentiality, term, general,
schedules). What remains is **(1)** filling bracketed placeholders, **(2)** a set
of substantive decisions, and **(3)** reconciling two cross-dependencies with the
rest of the v1 work.

---

## 1. Placeholders to complete

Every `[BRACKETED]` value in `COMMERCIAL-AGREEMENT.md`:

| § | Placeholder | Decision needed |
|---|---|---|
| Preamble | `[JURISDICTION OF INCORPORATION]`, `[COMPANY NUMBER]`, `[REGISTERED ADDRESS]` | Teqpace's registration details |
| 2.1 | `[non-exclusive / non-transferable / worldwide]` | Grant nature |
| 2.3 | `[Affiliates may / may not]` exercise the license | Affiliate rights |
| 4.2 | Fees `[exclusive / inclusive]` of taxes | Tax treatment |
| 4.3 | Payment term `[30]` days; interest `[statutory rate / LATE-FEE RATE]` | Payment terms |
| 5.3 | Update entitlement `[as set out in the Order Form]` | Confirm or specify |
| 7.1 | Limited-warranty period `[90]` days | Warranty window + remedy |
| 9.2 | Liability cap window `[12]` months | Cap basis |
| 9.3 | Carve-outs from the liability cap | Confirm list (death/PI, fraud, indemnity, payment) |
| 11.2 | Cure period `[30]` days | Termination terms |
| 12.2 | `[GOVERNING LAW]`, `[VENUE]`, `[exclusive]` jurisdiction | Governing law & venue |
| Signatures | `[LICENSEE LEGAL NAME]`, `[NAME]`, `[TITLE]`, `[DATE]` | Per-deal (Order Form) |
| Schedule A | Licensed Software version/components & form | Per-deal |
| Schedule B | Pricing model, amounts, currency, schedule | Pricing strategy |
| Schedule C | Support tier, hours, response/restore SLA, escalation | Support strategy |

---

## 2. Substantive decisions for counsel

- [ ] **Grant scope (§2):** exclusivity, transferability, worldwide; whether
      "internally modify" + object-form distribution + SaaS operation match the
      intended business model.
- [ ] **Affiliate rights (§2.3).**
- [ ] **Fee/pricing model (Schedule B):** per-instance / per-environment /
      per-transaction-volume / annual subscription.
- [ ] **Liability cap (§9.2):** multiple of fees and the look-back window;
      whether a higher cap should apply to the IP indemnity (see §3 below).
- [ ] **Warranty (§7.1):** period, conformance standard, exclusive remedy.
- [ ] **Indemnity scope (§8):** see the **B4 cross-dependency** in §3 — this is
      the item most affected by the clean-room provenance question.
- [ ] **Data protection:** Isopace is a framework and the Licensee controls
      cardholder/personal data, but confirm whether a DPA or a UK GDPR / GDPR
      carve-out/exclusion belongs here.
- [ ] **Export control & sanctions (§12.1):** Isopace ships cryptography
      (`vault`: PIN/MAC/DUKPT/TR-31/EMV). Confirm whether crypto export-control
      classification or notices are required for distribution.
- [ ] **Governing law & venue (§12.2).**
- [ ] **Open-source / inbound-rights chain:** confirm the
      [CLA](CLA.md) → commercial-license chain is airtight, i.e. Teqpace holds
      sufficient rights in **all** contributed code to grant this proprietary
      license (the basis of the dual-license model).

---

## 3. Cross-dependencies with the v1 roadmap

- [ ] **B4 (provenance) ↔ §8.1 Licensor IP indemnity.** Section 8.1 has Licensor
      *defend and indemnify* the Licensee against IP-infringement claims on the
      unmodified Licensed Software. That promise is only safe if every shipped
      package has **clean, defensible provenance**. The profile-provenance wording
      has been corrected to a clean-room/primary-sources basis (see
      [`ROADMAP-to-v1.md` B4](ROADMAP-to-v1.md)); counsel should confirm the
      indemnity is acceptable in light of the clean-room positioning, and consider
      whether internal provenance representations should back it.
- [ ] **B1 (HSM) ↔ §7.2 security/compliance disclaimer.** Section 7.2 already
      disclaims standalone PCI/EMV compliance and puts secure deployment
      (certified HSM for PIN/key ops) on the Licensee. This aligns with the B1
      plan to ship a certified-HSM `Vault` path; keep the disclaimer even after
      B1 lands (the framework is still not, by itself, a certified product).

---

## 4. Operational prerequisites (non-legal, but required before launch)

- [ ] Provision and monitor `licensing@teqpace.com` (referenced in
      [`COMMERCIAL-LICENSE.md`](COMMERCIAL-LICENSE.md)).
- [ ] Provision and monitor `security@teqpace.com` (referenced in
      [`SECURITY.md`](SECURITY.md)).
- [ ] Prepare an **Order Form** template (the per-deal document the agreement
      references for scope, fees, term, and support level).

---

## 5. Sign-off

- [ ] All placeholders completed.
- [ ] All §2 decisions made.
- [ ] §3 cross-dependencies reconciled (B4 resolved, B1 disclaimer retained).
- [ ] Counsel has reviewed, completed, and approved `COMMERCIAL-AGREEMENT.md`.
- [ ] The "⚠️ DRAFT — NOT LEGAL ADVICE" banner is removed from the final,
      counsel-approved version.
