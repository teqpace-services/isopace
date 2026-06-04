// Isopace Thales payShield (payment-HSM) Vault adapter — a separate module so the
// stdlib-only core never gains an HSM transport dependency. This module is itself
// stdlib-only (it depends only on the core module for the vault interfaces).
module github.com/teqpace-services/isopace/adapters/payshield

go 1.26

require github.com/teqpace-services/isopace v0.3.0

replace github.com/teqpace-services/isopace => ../..
