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

### teq — the container, assembled

`teq` is the Isopace container (the jPOS Q2 analog) packaged for easy startup. It
wraps `runtime.Host` and adds first-class **switch connections**: each `Switch`
is a self-healing `connector.Connector` the host supervises, so links to upstream
switches (Interswitch, UP, …) are dialled, kept alive, and reconnected on their
own. Starting from code is a few lines:

```go
q := teq.New()
isw, _ := q.Switch(connector.Config{Name: "isw", Addr: "isw.example:5000", Keyer: keyer})
up,  _ := q.Switch(connector.Config{Name: "up",  Addr: "up.example:6000",  Keyer: keyer})
q.Start(ctx)                                   // connectors dial in the background
resp, _ := q.To("isw").Request(ctx, frame)     // route by name; auto-reconnects
q.ListenAndServe()                             // or run as a daemon until SIGINT/SIGTERM
```

The `teq` example stands up two issuers as upstream switches and routes
transactions to each by name:

```sh
go run ./examples/teq
```

`connector.Config` exposes hooks for the switch-specific bits: `OnConnect` for
sign-on / key exchange after each (re)connect, and `Keepalive` for a periodic
echo (0800) that holds the link open and detects a dead peer. The underlying
`link` options carry the framer (e.g. a Postilion 2-byte length prefix), TLS, and
MAC filters.

#### Run it as a daemon from a deploy directory

`cmd/teq` is the container as a runnable — boot it from a directory of JSON
descriptors and it stays up, hot-(re)deploying as the files change (the Q2
experience). A switch descriptor fully specifies the link — packager, framer,
sign-on, echo, reconnect — so adding one needs no code:

```jsonc
// examples/teq/deploy/isw.json
{
  "name": "isw", "type": "connector", "enabled": true,
  "config": {
    "addr": "127.0.0.1:8583", "packager": "iso87-a", "framer": "length2",
    "sign_on": { "mti": "0800", "nmic": "001" },
    "echo":    { "mti": "0800", "nmic": "301", "interval_ms": 15000 },
    "header":  { "41": "TERM0001" }
  }
}
```

```sh
go run ./examples/simulator -addr 127.0.0.1:8583   # a switch to talk to (terminal 1)
go run ./cmd/teq -deploy ./examples/teq/deploy      # the container (terminal 2)
```

Inside the container, components find each other by name (`q.Get` / `q.To`) and
decouple through a shared tuple space (`q.Space()`) — e.g. a connector routes
server-initiated advices to a queue via its `unsolicited_queue`.

#### Routing + transforming gateway (the switch)

A `gateway.Gateway` is the inbound half: it listens for transactions, routes each
to an upstream via a `Forwarder` (a connector), and can transform the request on
the way out and the response on the way back — the store-and-forward heart of a
switch:

```go
q.Gateway(gateway.Config{
    Name: "pos", Addr: ":9000", Codec: c,
    Route: func(_ context.Context, req *iso8583.Message) (gateway.Forwarder, error) {
        return q.To("isw"), nil               // pick the destination switch
    },
    BeforeForward: func(_ context.Context, req *iso8583.Message) (*iso8583.Message, error) {
        _ = req.Set(32, acquirerID); return req, nil   // edit before forwarding
    },
    AfterForward: func(_ context.Context, _, resp *iso8583.Message) (*iso8583.Message, error) {
        _ = resp.Set(48, "ROUTED-VIA-TEQ"); return resp, nil  // edit the reply
    },
    OnError: declineOnError,                   // build a decline if routing fails
})
```

The `teqswitch` example shows the whole path — `client → gateway (transform) →
connector → host → gateway (transform) → client`:

```sh
go run ./examples/teqswitch
```

A pure routing gateway (no transforms) is also declarative — a `gateway`
descriptor with `route_to` a named connector — so `cmd/teq` can stand up an
inbound switch and its upstream links entirely from the deploy directory
(`examples/teq/deploy` has `pos` → `isw`). Transforms need code (`q.Gateway`).

### Component host

The `runtimehost` demo drives `runtime.Host` — the component container (the
jPOS Q2 analog) — on its own, with no network. It shows the start-in-order /
stop-in-reverse lifecycle, live `Deploy`/`Undeploy`, and a `Deployer` that turns
a directory of declarative JSON descriptors into running components and
hot-(re)deploys them as the files change.

```sh
go run ./examples/runtimehost
```

### Transaction flow

The `flowdemo` runs the `flow` package — the two-phase transaction manager — as a
small in-process issuer pipeline (no network). A prepare pass validates, routes by
BIN, and reserves funds; a commit pass then captures the funds, or an abort pass
releases the hold and carries a decline. Journaling, the per-stage profiler, and
idempotent retransmission are all wired in so each shows up in the output.

```sh
go run ./examples/flowdemo
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
