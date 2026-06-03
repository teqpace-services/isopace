# Isopace PKCS#11 (HSM) Vault adapter

A `vault.Vault` implementation backed by a PKCS#11 cryptographic token (an HSM),
in a **separate module** so the stdlib-only core never gains a cgo / PKCS#11
dependency. Keys are referenced by `CKA_LABEL`; key material never leaves the
token.

> ## ⚠️ Foundation only — not production-ready
>
> This module currently provides the **connection / session / key-lookup
> foundation** (build-verified). The four cryptographic `Vault` methods are
> **stubbed** (`ErrNotImplemented`) on purpose — see *Status* below. Nothing here
> is production-ready: a PKCS#11 Vault requires **independent security review and
> validation against a certified HSM** (PCI PIN Security / FIPS) before use.

## What works today

```go
v, err := pkcs11.Open(pkcs11.Config{
    ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
    TokenLabel: "isopace",
    PIN:        "1234",
})
// v satisfies vault.Vault; key lookup by CKA_LABEL is wired.
defer v.Close()
```

- Module load, token selection, session open, `C_Login`.
- Key lookup by label (`keyRef` → object handle).
- Compile-time `vault.Vault` conformance.

## Status of the cryptographic methods

| Method | Status | Intended PKCS#11 mapping |
|---|---|---|
| `GenerateMAC` / `VerifyMAC` | **stub** | `C_Sign`/`C_Verify` under a mechanism chosen from `(alg, pad)` — e.g. `CKM_DES3_MAC` for ISO 9797-1, `CKM_AES_CMAC` for CMAC. The mapping is HSM-specific and must be verified against a real token and security-reviewed. |
| `EncryptPINBlock` | **stub** | `vault.EncodePINBlock` + `C_Encrypt` (`CKM_DES3_ECB`). See PIN-security note. |
| `TranslatePIN` | **stub (blocked)** | No secure stock-PKCS#11 implementation exists — see below. |

## Why `TranslatePIN` is blocked (a `vault` API decision)

`vault.Vault.TranslatePIN` is contractually *decrypt under src → re-encode →
re-encrypt under dst*. On software (`SoftVault`) that is fine. On an HSM,
emulating it with `C_Decrypt`/`C_Encrypt` would materialise the **clear PIN block
in host memory**, which violates **PCI PIN Security**. A secure translate is a
single **atomic HSM operation** in which the clear PIN never leaves the token —
and that is a **vendor-specific** command (Thales, Futurex, …), **not** part of
standard PKCS#11.

Likewise, `EncryptPINBlock` takes a clear `pin string`, so the clear PIN is
already in host memory regardless of the HSM — appropriate for issuer/testing
flows, but not for an acquiring switch.

**Decision needed:** for the production-HSM path, should the `vault.Vault`
interface gain HSM-oriented operations that work on *encrypted* PIN blocks with
an atomic translate (so the clear PIN never transits the host), or should HSM PIN
translation live behind a separate, vendor-specific interface? This is tracked as
part of **B1** in [`ROADMAP-to-v1.md`](../../ROADMAP-to-v1.md) and is relevant to
the v1 API freeze of the `vault` package.

## Testing

The no-HSM tests (interface conformance, stub behaviour, config validation) run
anywhere. A functional suite against **SoftHSM2** is added once the operations
are implemented:

```sh
# CI (Ubuntu):
sudo apt-get install -y softhsm2
softhsm2-util --init-token --slot 0 --label isopace --pin 1234 --so-pin 5678
# import a 3DES test key, then: go test ./...
```

Building this module requires **cgo** (a C compiler) because PKCS#11 is a C ABI.
