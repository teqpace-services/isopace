# Isopace Thales payShield (payment-HSM) adapter

A reference adapter that exposes a **Thales payShield** payment HSM as the Isopace
vault capabilities a switch needs — `vault.PINTranslator` (PCI-secure PIN
translate) and `vault.Macer` (ISO 9797-1 message MAC) — over the payShield
**host-command protocol**. Separate module; this module is itself stdlib-only
(it depends only on the core for the `vault` interfaces).

> ## ⚠️ Scaffold — validated against a simulator, not hardware
>
> This adapter is a **scaffold**: it is exercised end to end against the in-repo
> [`Simulator`](#the-simulator-a-test-double) in CI, but it has **not** been run
> against a real payShield, real LMK-encrypted key tokens, or PCI-certified
> hardware. Two things are deliberately not yet real-device-faithful — see
> [What's real vs. what's a stand-in](#whats-real-vs-whats-a-stand-in). Treat it
> as the integration skeleton, pending validation against a genuine payShield (or
> Thales's own simulator) and independent security review.

## Why a payment HSM (the capability split)

A payment HSM performs **PIN translate atomically inside the device**, so the
clear PIN never reaches host memory (PCI PIN Security). That is exactly the
capability a general-purpose PKCS#11 HSM lacks — see [`adapters/pkcs11`](../pkcs11),
which implements only `vault.Macer` for that reason and deliberately omits
`vault.PINTranslator`. This adapter advertises **`vault.PINTranslator` and
`vault.Macer`**.

```go
v, err := payshield.Open(payshield.Config{Addr: "10.0.0.5:1500"})
if err != nil { /* ... */ }
defer v.Close()

// PCI-secure translate: the clear PIN never reaches the host.
newBlock, err := v.TranslatePIN("zpk-acquirer", "zpk-issuer", encBlock, pan,
    vault.ISO0, vault.ISO0)

mac, err := v.GenerateMAC("zak-1", vault.MACAlg3, vault.Pad1, msg) // retail MAC
```

Keys are named by the device's own **key token** (a key encrypted under the HSM's
Local Master Key); the clear key never leaves the device.

## Protocol

The host-command protocol is a TCP message framed by a 2-byte big-endian length
prefix over `header‖command`. Defaults (all overridable via `Config` to match
your firmware's *Host Command Reference Manual*):

| Operation | Request code | Response code | Notes |
|---|---|---|---|
| `TranslatePIN` | `CC` | `CD` | source/destination ZPK token, PIN-block format codes, PAN, source PIN block |
| `GenerateMAC` | `M6` | `M7` | key token, ISO 9797-1 algorithm + padding, message |
| `VerifyMAC` | `M8` | `M9` | …plus the MAC to check; verify-fail is a distinct error code |

The response code is the request code with its last character incremented; a
two-character error code (`00` = success) follows. A non-success code surfaces as
a `*payshield.HostError`. ISO 9564 PIN-block formats map to payShield format
codes via `Config.FormatCodes` (defaults: ISO0 `01`, ISO1 `05`, ISO3 `47`).

## The simulator (a test double)

`payshield.Simulator` is a TCP server that speaks the adapter's framing and
command set, so the adapter runs end to end without hardware. **Its cryptography
is the Isopace software vault** (`vault.SoftVault`), so the ISO 9564 / ISO 9797-1
values it returns are real — but it is **not** a payShield and **not** Thales's
simulator: there is no LMK, no key-token security, no PCI boundary. Keys are
loaded in the clear with `ImportKey`.

```go
sim, _ := payshield.NewSimulator(payshield.Config{})
defer sim.Close()
sim.ImportKey("zak-1", macKeyBytes)
v, _ := payshield.Open(payshield.Config{Addr: sim.Addr()})
```

The test suite asserts the adapter's MAC equals the `vault` software reference,
that an ISO0→ISO3→ISO0 translate round-trips, and that unknown keys / unsupported
algorithms surface correctly — all in-process, no external dependency.

## What's real vs. what's a stand-in

**Real / faithful:** the capability surface (`PINTranslator` + `Macer`, no
`PINEncryptor`); the host-command framing and request/response-code convention;
key-token references; PIN-block-format and MAC-algorithm selection; error-code
handling.

**Stand-in (the remaining integration work):**

- The on-wire **field layout** is a simplified, self-consistent encoding
  (length-prefixed fields) standing in for payShield's **positional** field
  structure. Swap it for the exact layout in the Host Command Reference for your
  firmware.
- The transport is a single connection serialised by a mutex; it does **not**
  pool or auto-reconnect. Front it with the supervised-connection machinery
  (`connector`) for production.
- No validation against real LMK schemes, key tokens, or certified hardware;
  independent security review is required before production use.

This is tracked under **B1** in [`ROADMAP-to-v1.md`](../../ROADMAP-to-v1.md).
