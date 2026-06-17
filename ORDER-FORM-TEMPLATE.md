<!--
SPDX-License-Identifier: LicenseRef-Teqpace-Commercial
Copyright (C) 2026 Teqpace Services Ltd. All rights reserved.
This is the per-deal Order Form template referenced by COMMERCIAL-AGREEMENT.md.
NOT offered under the AGPL.
-->

# Isopace Commercial License — Order Form

> ## ⚠️ DRAFT TEMPLATE — NOT LEGAL ADVICE
>
> This is an engineering-prepared **template** for the per-deal Order Form that
> [`COMMERCIAL-AGREEMENT.md`](COMMERCIAL-AGREEMENT.md) references (§1.3). It must
> be reviewed and approved by qualified legal counsel licensed in the governing
> jurisdiction (Nigeria — see [`COMMERCIAL-LICENSE-CHECKLIST.md`](COMMERCIAL-LICENSE-CHECKLIST.md) §6)
> before use. Every `[BRACKETED]` value is completed per deal.

This Order Form is governed by and incorporates the **Isopace Commercial License
Agreement** (the "**Agreement**"). Capitalized terms not defined here have the
meaning given in the Agreement. In a conflict, the order of precedence in
Agreement §12.6 applies (this Order Form, then the Agreement, then the Schedules).

---

## 1. Parties

| | Licensor | Licensee |
|---|---|---|
| Legal name | Teqpace Services Ltd. | `[LICENSEE LEGAL NAME]` |
| Registration | RC `[CAC RC NUMBER]` (Nigeria) | `[REG. NUMBER / JURISDICTION]` |
| Registered address | `[REGISTERED ADDRESS]` | `[LICENSEE ADDRESS]` |
| Notices contact | `licensing@teqpace.com` | `[LICENSEE CONTACT + EMAIL]` |

**Effective Date:** `[DATE]` (or the date of last signature below).

---

## 2. Licensed scope (defines the §2.1 grant boundary)

| Parameter | Value |
|---|---|
| Grant nature (§2.1) | `[non-exclusive / non-transferable / worldwide — per Agreement decision]` |
| Affiliates permitted (§2.3) | `[yes / no]` |
| Permitted use | `[internal use / embed in Licensee Application / SaaS operation — select]` |
| Environments | `[e.g. production + staging]` |
| Instances / nodes | `[count, or "unlimited within environments above"]` |
| Transaction-volume cap | `[e.g. N transactions/month, or "uncapped"]` |
| Named Licensee Application(s) | `[product/service name(s)]` |

Use outside this scope requires a new or amended Order Form (Agreement §3(d)).

---

## 3. Schedule A — Licensed Software

| | Value |
|---|---|
| Product | Isopace (`github.com/teqpace-services/isopace`) |
| Version / components | `[e.g. v1.0.0; core packages + adapters/sql + adapters/otel]` |
| Form delivered | `[source / object]` |
| Delivery method | Electronic (Agreement §5.1) |

> **Note (B1 / §7.2):** Production PIN and key handling require a certified HSM
> behind the `Vault` interface. The certified-HSM adapter and any HSM hardware are
> **not** included unless explicitly listed above. Secure deployment remains the
> Licensee's responsibility per Agreement §7.2.

---

## 4. Schedule B — Fees

| | Value |
|---|---|
| Pricing model | `[per-instance / per-environment / per-transaction-volume / annual subscription]` |
| Amount | `[amount]` |
| Currency | `[NGN / USD / other]` |
| Payment schedule | `[annual / quarterly / one-time]` |
| Taxes (§4.2) | `[exclusive / inclusive]` of applicable taxes (e.g. Nigerian VAT) |
| Payment term (§4.3) | `[30]` days from undisputed invoice |
| Late-payment interest (§4.3) | `[defined rate — e.g. CBN MPR + N% per annum]` |

---

## 5. Schedule C — Support & SLA

| | Value |
|---|---|
| Support purchased? | `[yes / no]` (if no, Agreement §5.2 — no support obligation) |
| Support tier | `[e.g. Standard / Priority]` |
| Support hours | `[e.g. 09:00–17:00 WAT, business days]` |
| Response target | `[e.g. P1 4h / P2 1 business day]` |
| Restore / workaround target | `[e.g. P1 1 business day]` |
| Escalation contact | `[name / email]` |
| Update entitlement (§5.3) | `[included during Term / as specified]` |

---

## 6. Term

| | Value |
|---|---|
| Initial Term (§11.1) | `[e.g. 12 months from Effective Date]` |
| Renewal | `[auto-renew / by mutual written agreement / none]` |
| Cure period (§11.2) | `[30]` days |

---

## 7. Signatures

| Licensor — Teqpace Services Ltd. | Licensee — `[LICENSEE LEGAL NAME]` |
|---|---|
| Signature: _______________________ | Signature: _______________________ |
| Name: `[NAME]` | Name: `[NAME]` |
| Title: `[TITLE]` | Title: `[TITLE]` |
| Date: `[DATE]` | Date: `[DATE]` |
