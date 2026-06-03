// Isopace PKCS#11 (HSM) Vault adapter — a separate module so the stdlib-only
// core never gains a cgo / PKCS#11 dependency. The miekg/pkcs11 requirement is
// filled in by `go mod tidy`.
module github.com/teqpace-services/isopace/adapters/pkcs11

go 1.26

require (
	github.com/miekg/pkcs11 v1.1.2
	github.com/teqpace-services/isopace v0.3.0
)

replace github.com/teqpace-services/isopace => ../..
