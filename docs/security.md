---
title: Security
description: How to report a vulnerability in Isopace, and the security-critical scope.
---

# Security policy

Isopace handles financial transaction data, so we take security seriously. The
authoritative policy is
[SECURITY.md](https://github.com/teqpace-services/isopace/blob/main/SECURITY.md)
in the repository.

## Reporting a vulnerability

!!! danger "Do not open public issues for security vulnerabilities"

    Report privately instead.

Report privately to:

- **Email:** `security@teqpace.com`
- **Website:** [teqpace.com](https://teqpace.com)
- **GitHub:** use private vulnerability reporting
  (**Security → Report a vulnerability** on the
  [repository](https://github.com/teqpace-services/isopace/security)).

Please include a description, reproduction steps, affected versions/commits, and
impact. We aim to acknowledge reports within a few business days and will keep you
updated on remediation.

## Supported versions

Isopace is pre-1.0; there are no supported release lines yet. This section will be
updated when the first stable release is published. See the
[versioning policy](versioning.md) for the stability promise.

## Scope

Cryptographic and key-management components ([`vault`](packages.md#vault)) are
security-critical. Please pay particular attention to PIN/MAC handling, key
storage, and any code that touches the wire format.

!!! warning "Use a certified HSM in production"

    The built-in software `Vault` backend is for development and testing only.
    Production PIN and key handling require a certified HSM.
