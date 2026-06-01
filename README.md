# Isopace

**Isopace** is a financial transaction framework for Go — ISO-8583 messaging and
payment switching, plus the runtime, transaction pipeline, coordination, and
security building blocks needed to run a production switch.

It is an independent, clean-room implementation in the spirit of
[jPOS](https://jpos.org), redesigned around idiomatic Go: type-safe field access,
zero-copy decoding, a pluggable codec catalog, and goroutine-native concurrency.

> **Status: pre-alpha / under active development.** APIs are unstable and will
> change. Not yet ready for production use.

A product of **[Teqpace Services Ltd.](https://teqpace.com)**.

---

## What Isopace aims to provide

| Layer | Component | Purpose |
|-------|-----------|---------|
| Messaging | `iso8583` — `Message`, `Field`, `Schema`, `Codec`, `FieldCodec` | Type-safe, zero-copy ISO-8583 marshal/unmarshal |
| Codecs | pluggable `FieldCodec` registry | ASCII / EBCDIC / BCD / binary, all jPOS `IF*` variants, BER-TLV, and more |
| Transport | `Link`, `Listener`, `Switch` | Connections, servers, and request↔response switching |
| Runtime | `Runtime` | Component host with lifecycle, configuration, and hot (re)deploy |
| Processing | `Flow` / `Stage`, `Exchange` | The transaction pipeline and its shared state |
| Coordination | `Space` | In-process and distributed tuple-space / store-and-forward |
| Security | `Vault` | PIN/MAC, key management, and HSM access |

The full design is documented in [`ARCHITECTURE.md`](ARCHITECTURE.md) *(in progress)*.

## Why not just use jPOS?

jPOS is excellent and battle-tested, but it is Java/AGPL and carries 25 years of
design history. Isopace is a **from-scratch Go implementation** that aims to keep
what makes jPOS great while improving on its data model (compile-time type safety,
zero-copy parsing, first-class validation, multi-format rendering) and operating
naturally in cloud-native, high-throughput Go services.

Isopace is **not** a translation or port of jPOS. It is built clean-room from the
ISO-8583 standard and public payments knowledge — no jPOS source is consulted.

## Licensing

Isopace is **dual-licensed**:

- **Open source:** [GNU AGPL v3.0 or later](LICENSE). If you run a modified
  Isopace as a network service, AGPL section 13 requires you to offer your users
  the corresponding source.
- **Commercial:** A proprietary license from Teqpace Services Ltd. removes the
  AGPL copyleft obligations for closed-source or hosted/commercial deployments.
  See [COMMERCIAL-LICENSE.md](COMMERCIAL-LICENSE.md).

## Contributing

Contributions are welcome. External contributors must sign the
[Contributor License Agreement](CLA.md) so that Teqpace can maintain the
dual-license model. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Please report vulnerabilities responsibly — see [SECURITY.md](SECURITY.md).
