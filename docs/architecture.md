---
title: Architecture
description: >-
  The implementation-ready design of the Isopace core — the in-memory Message
  model, schema, codec engine, zero-copy decode, validation, and alternate
  renderings.
---

!!! abstract "About this document"

    This is the canonical design specification for the **Isopace core** — the
    `Message` model, `Schema`, the per-field wire engine, the marshal/unmarshal
    `Codec`, struct binding, validation, and alternate renderings. It is rendered
    verbatim from
    [`ARCHITECTURE.md`](https://github.com/teqpace-services/isopace/blob/main/ARCHITECTURE.md)
    in the repository, so it always matches the source of truth.

    New to Isopace? Start with the [Getting Started](getting-started.md) guide,
    then come back here for the design rationale. The per-package API reference
    lives on
    [pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace).

--8<-- "ARCHITECTURE.md"
