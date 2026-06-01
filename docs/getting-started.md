# Getting started

Isopace is a Go library plus a set of example programs. This guide runs the
examples and shows the core API. The full design is in
[`ARCHITECTURE.md`](../ARCHITECTURE.md); the per-package API reference is the
GoDoc (`go doc ./...` or pkg.go.dev).

## Requirements

- Go matching the version in [`go.mod`](../go.mod).
- No other dependencies — Isopace is stdlib-only.

```sh
git clone https://github.com/teqpace-services/isopace
cd isopace
go test ./...
```

## Run the examples

The examples implement a tiny acquirer ↔ issuer authorization flow over TCP.

**Terminal 1 — issuer** (answers 0200 with 0210, approving up to a limit):

```sh
go run ./examples/issuer -addr 127.0.0.1:8583 -limit 10000
```

**Terminal 2 — acquirer** (sends authorizations through a switch and prints
responses):

```sh
go run ./examples/acquirer -addr 127.0.0.1:8583 -n 5 -amount 2500
# stan 1 -> 0210 rc=00 auth="000001"
# ...
```

### Simulator / test host

The `simulator` assembles a small switch from the framework's building blocks: a
`runtime.Host` supervises an ISO-8583 listener and an admin HTTP endpoint, with
the metrics registry wired in as the runtime observer.

```sh
go run ./examples/simulator -addr 127.0.0.1:8583 -admin 127.0.0.1:8584 -limit 10000
curl http://127.0.0.1:8584/healthz   # liveness
curl http://127.0.0.1:8584/readyz    # readiness (health checks)
curl http://127.0.0.1:8584/metrics   # Prometheus exposition
```

## Core API in a nutshell

Build and encode a message:

```go
s := packager.ISO87A()
c := iso8583.NewCodec(s)

m := iso8583.New(s)
_ = m.Set(0, "0200")
_ = m.Set(11, int64(123456))   // STAN
_ = m.Set(41, "TERM0001")      // terminal
_ = m.Set(4, int64(2500))      // amount, minor units

wire, _ := c.Marshal(m, nil)
```

Decode and read fields with compile-time types (no casting):

```go
got, _ := c.Unmarshal(wire)
mti, _ := iso8583.Get[string](got, 0)
stan, _ := iso8583.Get[int64](got, 11)
```

Switch request/response over a link, correlating by STAN + terminal:

```go
cl, _ := link.Dial("tcp", "127.0.0.1:8583")
x := mux.New(cl, mux.FieldKeyer(c, 11, 41), mux.WithTimeout(5*time.Second))
resp, _ := x.Request(context.Background(), wire)
```

The shared logic behind the examples lives in
[`examples/posdemo`](../examples/posdemo), with an end-to-end loopback test you
can read as a worked example.

## Where to go next

- [`ARCHITECTURE.md`](../ARCHITECTURE.md) — the design and the layering rules.
- [`docs/versioning.md`](versioning.md) — the SemVer policy and stability promise.
- [`CHANGELOG.md`](../CHANGELOG.md) — what shipped in each release.
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — the clean-room rule, SPDX headers, CLA.
