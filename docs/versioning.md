# Versioning policy

Isopace follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html):
releases are tagged `vMAJOR.MINOR.PATCH`.

## Pre-1.0 (the `0.x` series)

Isopace is currently in the `0.x` series. Under SemVer, this means the public API
is **not yet stable**:

- **`0.MINOR.0`** (minor bumps) may contain breaking API changes. Each is
  described in [`CHANGELOG.md`](../CHANGELOG.md), with migration notes when a
  change is not source-compatible.
- **`0.x.PATCH`** (patch bumps) are bug fixes and additive, source-compatible
  changes only.

Pin a specific version in `go.mod` and review the changelog before upgrading a
minor. Treat the API as evolving until `v1.0.0`.

## 1.0.0 and after

From `v1.0.0` the public API of every non-`internal`, non-`examples` package is
stable within a major version:

- **MAJOR** — incompatible API changes.
- **MINOR** — backward-compatible additions.
- **PATCH** — backward-compatible bug fixes.

## What "public API" covers

The compatibility promise applies to exported identifiers in the importable
packages (`iso8583`, `fieldcodec`, `lengthcodec`, `packager`, `render/*`, `link`,
`listener`, `mux`, `runtime`, `flow`, `space`, `vault`, `rbac`, `store`, `ops`).

It does **not** cover:

- packages under `internal/` (not importable);
- the `conformance` package (tests, golden vectors, and benchmarks only — no
  shipped runtime API);
- the `examples/` programs (illustrative, not API);
- on-the-wire encodings beyond what a profile documents, log output, error
  message text, or unexported behaviour;
- the `tools/` directory.

## Go version

Isopace targets the Go version declared in [`go.mod`](../go.mod). The minimum
supported Go version may be raised in a minor release; that bump is noted in the
changelog.

## Dependencies

Isopace is **stdlib-only**: the module graph carries no third-party dependency,
which keeps the dual-license (AGPL-3.0 / commercial) model free of transitive
copyleft. Optional integrations (OpenTelemetry, NATS/JetStream, a SQL `Store`, an
HSM/PKCS#11 `Vault`) are designed as drop-in adapters in separate, optionally
imported modules so this property is preserved.
