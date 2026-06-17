---
title: Examples
description: >-
  The runnable example programs that ship with Isopace — a tiny acquirer/issuer
  flow, a simulator switch, the teq container, a transforming gateway, and more.
---

# Examples

Isopace ships a set of runnable programs under
[`examples/`](https://github.com/teqpace-services/isopace/tree/main/examples)
that assemble the package's building blocks into working switches and demos.
Each is small enough to read end-to-end. None require anything beyond the Go
toolchain — Isopace is stdlib-only.

```sh
git clone https://github.com/teqpace-services/isopace
cd isopace
go test ./...
```

## At a glance

| Program | What it demonstrates | Run |
|---------|----------------------|-----|
| [`issuer`](#issuer-and-acquirer) | Answers `0200` authorizations with `0210`, approving up to a limit | `go run ./examples/issuer` |
| [`acquirer`](#issuer-and-acquirer) | Sends authorizations through a switch and prints the responses | `go run ./examples/acquirer` |
| [`simulator`](#simulator) | A small switch assembled from a `runtime.Host` + listener + admin endpoint | `go run ./examples/simulator` |
| [`teq`](#teq-the-container) | The Isopace container with self-healing upstream `Switch` connections | `go run ./examples/teq` |
| [`teqswitch`](#teqswitch-a-transforming-gateway) | A routing and transforming gateway: client → gateway → connector → host | `go run ./examples/teqswitch` |
| [`runtimehost`](#runtimehost) | `runtime.Host` lifecycle, live deploy/undeploy, directory hot-redeploy | `go run ./examples/runtimehost` |
| [`flowdemo`](#flowdemo) | The two-phase `flow` transaction manager as an in-process issuer pipeline | `go run ./examples/flowdemo` |
| [`posdemo`](https://github.com/teqpace-services/isopace/tree/main/examples/posdemo) | Shared POS logic with an end-to-end loopback test — read as a worked example | _(library)_ |
| [`playground`](https://github.com/teqpace-services/isopace/tree/main/examples/playground) | A scratch space for trying the API | `go run ./examples/playground` |

!!! tip "Where the detail lives"

    The [Getting Started](getting-started.md) guide walks through most of these
    with full output and the code behind them. This page is the quick catalog.

## issuer and acquirer

The two programs implement a tiny acquirer ↔ issuer authorization flow over TCP.

=== "Terminal 1 — issuer"

    Answers `0200` with `0210`, approving up to a limit:

    ```sh
    go run ./examples/issuer -addr 127.0.0.1:8583 -limit 10000
    ```

=== "Terminal 2 — acquirer"

    Sends authorizations through a switch and prints responses:

    ```sh
    go run ./examples/acquirer -addr 127.0.0.1:8583 -n 5 -amount 2500
    # stan 1 -> 0210 rc=00 auth="000001"
    # ...
    ```

[:octicons-arrow-right-24: Run the examples](getting-started.md#run-the-examples)

## simulator

The `simulator` assembles a small switch from the package's building blocks: a
`runtime.Host` supervises an ISO-8583 listener and an admin HTTP endpoint, with
the metrics registry wired in as the runtime observer.

```sh
go run ./examples/simulator -addr 127.0.0.1:8583 -admin 127.0.0.1:8584 -limit 10000
curl http://127.0.0.1:8584/healthz   # liveness
curl http://127.0.0.1:8584/readyz    # readiness (health checks)
curl http://127.0.0.1:8584/metrics   # Prometheus exposition
```

[:octicons-arrow-right-24: Simulator / test host](getting-started.md#simulator--test-host)

## teq, the container

`teq` is the Isopace container (the jPOS Q2 analog) packaged for easy startup. It
wraps `runtime.Host` and adds first-class **switch connections**: each `Switch`
is a self-healing `connector.Connector` the host supervises.

```sh
go run ./examples/teq
```

It can also run as a daemon from a directory of JSON descriptors via `cmd/teq`,
hot-(re)deploying as the files change:

```sh
go run ./examples/simulator -addr 127.0.0.1:8583   # a switch to talk to (terminal 1)
go run ./cmd/teq -deploy ./examples/teq/deploy      # the container (terminal 2)
```

[:octicons-arrow-right-24: teq, assembled](getting-started.md#teq--the-container-assembled)

## teqswitch, a transforming gateway

A `gateway.Gateway` listens for transactions, routes each to an upstream via a
`Forwarder`, and can transform the request on the way out and the response on the
way back — the store-and-forward heart of a switch. `teqswitch` shows the whole
path `client → gateway (transform) → connector → host → gateway (transform) →
client`, and prints a correlated lifecycle [`trace`](getting-started.md#per-transaction-lifecycle-trace)
per transaction.

```sh
go run ./examples/teqswitch
```

[:octicons-arrow-right-24: Routing + transforming gateway](getting-started.md#routing--transforming-gateway-the-switch)

## runtimehost

The `runtimehost` demo drives `runtime.Host` — the component container — on its
own, with no network. It shows the start-in-order / stop-in-reverse lifecycle,
live `Deploy`/`Undeploy`, and a `Deployer` that turns a directory of declarative
JSON descriptors into running components and hot-(re)deploys them as the files
change.

```sh
go run ./examples/runtimehost
```

[:octicons-arrow-right-24: Component host](getting-started.md#component-host)

## flowdemo

The `flowdemo` runs the `flow` package — the two-phase transaction manager — as a
small in-process issuer pipeline (no network). A prepare pass validates, routes by
BIN, and reserves funds; a commit pass captures the funds, or an abort pass
releases the hold and carries a decline. Journaling, the per-stage profiler, and
idempotent retransmission are all wired in.

```sh
go run ./examples/flowdemo
```

[:octicons-arrow-right-24: Transaction flow](getting-started.md#transaction-flow)
