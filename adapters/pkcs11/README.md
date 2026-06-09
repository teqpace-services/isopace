# Isopace PKCS#11 (HSM) MAC adapter

A `vault.Macer` implementation backed by a PKCS#11 cryptographic token (a
general-purpose HSM), in a **separate module** so the stdlib-only core never
gains a cgo / PKCS#11 dependency. Keys are referenced by `CKA_LABEL`; key
material never leaves the token.

> ## ⚠️ Capability: MAC only — and review before production
>
> A general-purpose PKCS#11 HSM can compute MACs but **cannot** perform a
> PCI-compliant PIN translate, so this adapter implements **`vault.Macer` and
> nothing else**. It deliberately does **not** implement `vault.PINTranslator`
> or `vault.PINEncryptor` (see *Capability surface* below). The MAC path is
> functional and cross-checked against the `vault` software reference under
> SoftHSM2 in CI, but it still needs **independent security review and validation
> against your specific certified HSM** (PCI PIN Security / FIPS) before use.

## What it does

```go
v, err := pkcs11.Open(pkcs11.Config{
    ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
    TokenLabel: "isopace",
    PIN:        "1234",
})
if err != nil { /* ... */ }
defer v.Close()

// v is a vault.Macer. Key material never leaves the token.
mac, err := v.GenerateMAC("zak-1", vault.MACAlg3, vault.Pad1, msg) // retail MAC
ok, err := v.VerifyMAC("zak-1", vault.MACAlg3, vault.Pad1, msg, mac[:4])
```

- Module load, token selection, session open, `C_Login`; key lookup by
  `CKA_LABEL`.
- `GenerateMAC` / `VerifyMAC` for ISO 9797-1 **algorithm 1** (single-DES
  CBC-MAC) and **algorithm 3** (ANSI X9.19 retail MAC).
- Output is the full 8-byte MAC, **byte-for-byte identical to
  `vault.GenerateMAC`** (the CI suite asserts this against a real token), so a
  software peer and an HSM peer interoperate. Callers truncate as agreed.

## How the MAC is computed (and why)

Modern tokens disable single-DES and expose **no retail-MAC mechanism**
(`CKM_DES3_MAC` is a *full-3DES* CBC-MAC — a different algorithm). So the MAC is
composed from the 3DES primitives the token does provide (`CKM_DES3_CBC`,
`CKM_DES3_ECB`). The ISO 9797-1 padding and the message are not secret, so the
padding is applied in the host; every keyed block operation runs on the token
and the key never leaves it. A single-DES operation `E_K` is obtained by
presenting `K` as a triple-length key `K‖K‖K` (3DES-EDE then collapses to `E_K`).

| Algorithm | Computation | Key object(s) under `keyRef` |
|---|---|---|
| `MACAlg1` | final block of `CKM_DES3_CBC` | a `CKK_DES3` key `K‖K‖K` |
| `MACAlg3` | single-DES CBC-MAC under K1 over all but the last block, then a 3DES-EDE of `(lastBlock ⊕ prefixMAC)` | the natural retail key `K1‖K2‖K1` at `keyRef`, **plus** a `K1‖K1‖K1` helper at `keyRef`+`RetailCBCLabelSuffix` (default `-cbc`) for the CBC stage |

This needs the MAC key usable for 3DES **encrypt**. A hardened device that
restricts MAC keys to `CKA_SIGN` with a native MAC/CMAC mechanism should use that
mechanism instead (wire it in per device) — or use a payment-HSM adapter.

## Capability surface (the security property)

The adapter type satisfies `vault.Macer` and **must not** satisfy
`vault.PINTranslator` or `vault.PINEncryptor`. A switch checks for
`vault.PINTranslator` before trusting a vault with PINs; this adapter omits the
method entirely so that check correctly fails. Why no PIN ops here:

- **`TranslatePIN`** must be a single atomic operation inside the device so the
  clear PIN never reaches host memory. Standard PKCS#11 has no such mechanism;
  emulating it with `C_Decrypt` → re-encode → `C_Encrypt` would materialise the
  clear PIN block in the host and violate **PCI PIN Security**. Per the
  `vault.PINTranslator` contract, an adapter that cannot translate PIN-securely
  **must not advertise the interface**. Use a payment-HSM adapter (e.g. payShield)
  for PIN translation.
- **`EncryptPINBlock`** takes a *clear* PIN, so it belongs to an issuer /
  trusted-context adapter, not this stock-PKCS#11 one.

This is tracked under **B1** in [`ROADMAP-to-v1.md`](../../ROADMAP-to-v1.md).

## Testing

The no-HSM tests (capability surface, padding vectors, config validation) run
anywhere. The functional suite runs only when a token is configured via the
environment, and **cross-checks the adapter's MAC against the `vault` software
reference** for both algorithms, both paddings, and a range of message lengths,
plus verify/tamper:

```sh
# Provision a SoftHSM2 token (CI does this on Ubuntu via `apt-get install softhsm2`):
export SOFTHSM2_CONF=/path/to/softhsm2.conf      # tokendir must be writable
softhsm2-util --init-token --slot 0 --label isopace --pin 1234 --so-pin 5678

# Point the suite at the module; the test provisions/destroys its own keys:
ISOPACE_SOFTHSM_MODULE=/usr/lib/softhsm/libsofthsm2.so \
ISOPACE_SOFTHSM_TOKEN=isopace ISOPACE_SOFTHSM_PIN=1234 \
  go test -race ./...
```

Building this module requires **cgo** (a C compiler) because PKCS#11 is a C ABI.
