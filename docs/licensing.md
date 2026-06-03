---
title: Licensing
description: >-
  Isopace is dual-licensed under the GNU AGPL-3.0-or-later and a commercial
  license from Teqpace Services Ltd.
---

# Licensing

Isopace is **dual-licensed**. You may use it under whichever of the two licenses
fits your deployment.

<div class="grid cards" markdown>

-   :material-open-source-initiative:{ .lg .middle } **Open source — AGPL-3.0-or-later**

    ---

    The [GNU Affero General Public License v3.0 or later](https://github.com/teqpace-services/isopace/blob/main/LICENSE).
    If you run a modified Isopace as a network service, **AGPL section 13**
    requires you to offer your users the corresponding source.

    [:octicons-arrow-right-24: Read the LICENSE](https://github.com/teqpace-services/isopace/blob/main/LICENSE)

-   :material-briefcase:{ .lg .middle } **Commercial license**

    ---

    A proprietary license from Teqpace Services Ltd. removes the AGPL copyleft
    obligations for closed-source or hosted/commercial deployments.

    [:octicons-arrow-right-24: Commercial license](https://github.com/teqpace-services/isopace/blob/main/COMMERCIAL-LICENSE.md)

</div>

## Why stdlib-only matters here

Keeping the module **stdlib-only** is deliberate: with no third-party dependency
in the module graph, the dual-license model stays free of transitive copyleft.
Optional integrations (OpenTelemetry, NATS/JetStream, a SQL `Store`, an
HSM/PKCS#11 `Vault`) are designed as drop-in adapters in separate, optionally
imported modules so this property is preserved.

## Contributions

External contributors sign a [Contributor License Agreement](contributing.md) so
Teqpace can maintain the dual-license model. You keep copyright in your work; the
CLA is a license grant, not an assignment.

## Summary

| | AGPL-3.0-or-later | Commercial |
|---|---|---|
| Cost | Free | Paid |
| Source disclosure (incl. network use, AGPL §13) | Required | Not required |
| Closed-source / proprietary product | ✗ | ✓ |
| Hosted/SaaS without source offer | ✗ | ✓ |
| Support terms | Community | Per agreement |

!!! note

    This page is a summary, not legal advice. The authoritative terms are in
    [LICENSE](https://github.com/teqpace-services/isopace/blob/main/LICENSE) and
    [COMMERCIAL-LICENSE.md](https://github.com/teqpace-services/isopace/blob/main/COMMERCIAL-LICENSE.md).
    For commercial licensing, contact [Teqpace Services Ltd.](https://teqpace.com)
