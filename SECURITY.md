# Security Policy

Isopace handles financial transaction data, so we take security seriously.

## Reporting a vulnerability

**Please do not open public issues for security vulnerabilities.**

Report privately to:

- Email: `security@teqpace.com` *(confirm this mailbox is provisioned)*
- Website: https://teqpace.com
- Or use GitHub's private vulnerability reporting (Security → Report a vulnerability).

Please include a description, reproduction steps, affected versions/commits, and
impact. We aim to acknowledge reports within a few business days and will keep you
updated on remediation.

## Supported versions

Isopace is pre-alpha; there are no supported release lines yet. This section will
be updated when the first stable release is published.

## Scope

Cryptographic and key-management components (`Vault`) are security-critical.
Please pay particular attention to PIN/MAC handling, key storage, and any code
that touches the wire format.
