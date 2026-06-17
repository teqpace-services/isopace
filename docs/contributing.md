---
title: Contributing
description: >-
  How to contribute to Isopace — the Contributor License Agreement, the
  clean-room rule, source headers, and the development workflow.
---

# Contributing

Contributions are welcome. Isopace is a product of
**[Teqpace Services Ltd.](https://teqpace.com)** The authoritative documents are
in the repository:
[CONTRIBUTING.md](https://github.com/teqpace-services/isopace/blob/main/CONTRIBUTING.md)
and [CLA.md](https://github.com/teqpace-services/isopace/blob/main/CLA.md).

## Contributor License Agreement (required)

Isopace is dual-licensed (AGPL-3.0 + commercial). To keep that model legally
sound, **Teqpace must hold sufficient rights in all contributed code.**

- **External contributors** must agree to the
  [CLA](https://github.com/teqpace-services/isopace/blob/main/CLA.md) before
  their first contribution is merged. The CLA is a broad, irrevocable,
  sublicensable copyright + patent license grant — **you keep copyright in your
  work**; you do not assign it away.
- **Teqpace employees** contribute work made for hire already owned by Teqpace
  and do not need to sign the CLA.

A contribution made available *only* under the AGPL, without the CLA grant, would
prevent Teqpace from offering the commercial license — so this is non-negotiable
for inbound code.

## Clean-room rule

Isopace is an **independent, clean-room** implementation. Do **not** read, port,
translate, or copy from jPOS (or any other AGPL/GPL) source code. Implement only
from:

- the ISO-8583 standard and card-scheme / network specifications,
- public documentation and observable behaviour,
- your own design.

If you need to understand a specific behaviour, validate Isopace's output against
a reference implementation as a **black box** (compare wire bytes) — never copy
its code or test fixtures.

## Source file header

Every source file must begin with the SPDX dual-license header (Go shown):

```go
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction package.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.
```

Copyright is held by **Teqpace Services Ltd.** (not the individual author). Add
yourself to [AUTHORS](https://github.com/teqpace-services/isopace/blob/main/AUTHORS)
in the same change.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

- Target Go 1.26+.
- Keep packages idiomatic; prefer `context.Context`, typed errors, and
  table-driven tests.
- New `FieldCodec`s and packager profiles must ship with conformance tests.
- Run `gofmt`/`goimports`; CI enforces formatting, vet, tests, and a dependency
  license scan (no GPL/AGPL/LGPL/SSPL dependencies may enter the module graph).

## Pull requests

- Keep changes focused and described.
- Reference the relevant section of the [Architecture guide](architecture.md)
  where applicable.
- All CI checks (including the license scan) must pass.
