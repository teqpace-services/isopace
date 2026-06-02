// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction framework.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package teq

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/gateway"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/runtime"
)

// netMgmt describes a network-management exchange (sign-on or echo) declaratively.
type netMgmt struct {
	MTI        string `json:"mti"`         // default "0800"
	NMIC       string `json:"nmic"`        // DE 70 network-management info code (e.g. "001", "301")
	IntervalMS int    `json:"interval_ms"` // echo only: how often to send
}

// connectorDesc is the JSON config block of a "connector" descriptor — enough to
// fully describe an outbound switch link without code.
type connectorDesc struct {
	Addr             string            `json:"addr"`
	Network          string            `json:"network"`           // default "tcp"
	Packager         string            `json:"packager"`          // profile id, default "iso87-a"
	Framer           string            `json:"framer"`            // "length2" (default) | "length4"
	TLS              bool              `json:"tls"`               // enable TLS with the host as server name
	CorrelationDEs   []int             `json:"correlation_des"`   // default [11, 41]
	TimeoutMS        int               `json:"timeout_ms"`        // per-request timeout
	MinBackoffMS     int               `json:"min_backoff_ms"`    //
	MaxBackoffMS     int               `json:"max_backoff_ms"`    //
	Header           map[string]string `json:"header"`            // static DEs (e.g. "41","42") for sign-on/echo
	SignOn           *netMgmt          `json:"sign_on"`           // 0800 sent on each (re)connect
	Echo             *netMgmt          `json:"echo"`              // 0800 sent on echo.interval_ms
	UnsolicitedQueue string            `json:"unsolicited_queue"` // space key for server-initiated frames
}

// connectorFactory builds a switch connector from a descriptor, resolving the
// packager profile, framer, and sign-on/echo network-management exchanges by
// name/value — so a deploy descriptor fully specifies the link.
func (q *Q) connectorFactory(name string, cfg json.RawMessage, env *runtime.Env) (runtime.Component, error) {
	d := connectorDesc{Network: "tcp", Packager: "iso87-a", Framer: "length2", CorrelationDEs: []int{11, 41}}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &d); err != nil {
			return nil, fmt.Errorf("teq: connector %q config: %w", name, err)
		}
	}
	if d.Addr == "" {
		return nil, fmt.Errorf("teq: connector %q: addr required", name)
	}

	schema, ok := packager.Profile(d.Packager)
	if !ok {
		return nil, fmt.Errorf("teq: connector %q: unknown packager %q", name, d.Packager)
	}
	codec := iso8583.NewCodec(schema)

	framer, err := framerByName(d.Framer)
	if err != nil {
		return nil, fmt.Errorf("teq: connector %q: %w", name, err)
	}
	linkOpts := []link.Option{link.WithFramer(framer)}
	if d.TLS {
		linkOpts = append(linkOpts, link.WithTLS(&tls.Config{ServerName: hostOf(d.Addr)}))
	}

	cc := connector.Config{
		Name:        name,
		Network:     d.Network,
		Addr:        d.Addr,
		Keyer:       mux.FieldKeyer(codec, d.CorrelationDEs...),
		LinkOptions: linkOpts,
		Timeout:     msDur(d.TimeoutMS),
		MinBackoff:  msDur(d.MinBackoffMS),
		MaxBackoff:  msDur(d.MaxBackoffMS),
		Log:         env.Logger(),
	}

	// A per-connector STAN sequence drives DE 11 on sign-on / echo messages.
	stan := new(atomic.Uint64)
	if d.SignOn != nil {
		cc.OnConnect = q.netMgmtHook(codec, schema, *d.SignOn, d.Header, stan)
	}
	if d.Echo != nil {
		cc.KeepaliveInterval = msDur(d.Echo.IntervalMS)
		cc.Keepalive = q.netMgmtHook(codec, schema, *d.Echo, d.Header, stan)
	}
	if d.UnsolicitedQueue != "" {
		queue := d.UnsolicitedQueue
		sp := q.space
		cc.Unsolicited = func(frame []byte) { sp.Out(queue, frame) }
	}

	c, err := connector.New(cc)
	if err != nil {
		return nil, err
	}
	q.Put(name, c)
	return c, nil
}

// netMgmtHook builds a hook that sends one network-management (0800) message and
// waits for its correlated response — used for both sign-on (OnConnect) and echo
// (Keepalive).
func (q *Q) netMgmtHook(codec *iso8583.Codec, schema *iso8583.Schema, nm netMgmt, header map[string]string, stan *atomic.Uint64) func(context.Context, *mux.Mux) error {
	mti := nm.MTI
	if mti == "" {
		mti = "0800"
	}
	return func(ctx context.Context, m *mux.Mux) error {
		frame, err := buildNetMgmt(codec, schema, mti, nm.NMIC, header, stan)
		if err != nil {
			return err
		}
		_, err = m.Request(ctx, frame)
		return err
	}
}

// buildNetMgmt marshals a 0800 with a fresh STAN, the optional DE 70 NMIC, and
// any static header DEs.
func buildNetMgmt(codec *iso8583.Codec, schema *iso8583.Schema, mti, nmic string, header map[string]string, stan *atomic.Uint64) ([]byte, error) {
	m := iso8583.New(schema)
	if err := m.Set(0, mti); err != nil {
		return nil, err
	}
	if err := m.Set(11, int64(stan.Add(1)%1_000_000)); err != nil {
		return nil, err
	}
	if nmic != "" {
		n, err := strconv.ParseInt(nmic, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("nmic %q: %w", nmic, err)
		}
		if err := m.Set(70, n); err != nil {
			return nil, err
		}
	}
	for deStr, v := range header {
		de, err := strconv.Atoi(deStr)
		if err != nil {
			return nil, fmt.Errorf("header key %q is not a DE number", deStr)
		}
		if err := m.Set(de, v); err != nil {
			return nil, fmt.Errorf("header DE %d: %w", de, err)
		}
	}
	return codec.Marshal(m, nil)
}

// gatewayDesc is the JSON config block of a "gateway" descriptor — a pure
// routing switch server that forwards every inbound message to a named connector.
// Request/response transforms require code (see [Q.Gateway]).
type gatewayDesc struct {
	Addr      string `json:"addr"`
	Network   string `json:"network"`  // default "tcp"
	Packager  string `json:"packager"` // profile id, default "iso87-a"
	Framer    string `json:"framer"`   // "length2" (default) | "length4"
	RouteTo   string `json:"route_to"` // name of the connector to forward to
	TimeoutMS int    `json:"timeout_ms"`
}

// gatewayFactory builds a routing gateway from a descriptor. It forwards every
// inbound message to the connector named route_to, resolved lazily so the order
// of deployment does not matter.
func (q *Q) gatewayFactory(name string, cfg json.RawMessage, env *runtime.Env) (runtime.Component, error) {
	d := gatewayDesc{Network: "tcp", Packager: "iso87-a", Framer: "length2"}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &d); err != nil {
			return nil, fmt.Errorf("teq: gateway %q config: %w", name, err)
		}
	}
	if d.Addr == "" {
		return nil, fmt.Errorf("teq: gateway %q: addr required", name)
	}
	if d.RouteTo == "" {
		return nil, fmt.Errorf("teq: gateway %q: route_to required", name)
	}
	schema, ok := packager.Profile(d.Packager)
	if !ok {
		return nil, fmt.Errorf("teq: gateway %q: unknown packager %q", name, d.Packager)
	}
	framer, err := framerByName(d.Framer)
	if err != nil {
		return nil, fmt.Errorf("teq: gateway %q: %w", name, err)
	}
	routeTo := d.RouteTo
	g, err := gateway.New(gateway.Config{
		Name:        name,
		Network:     d.Network,
		Addr:        d.Addr,
		Codec:       iso8583.NewCodec(schema),
		LinkOptions: []link.Option{link.WithFramer(framer)},
		Route: func(context.Context, *iso8583.Message) (gateway.Forwarder, error) {
			dest := q.To(routeTo)
			if dest == nil {
				return nil, fmt.Errorf("gateway %q: route_to %q not connected", name, routeTo)
			}
			return dest, nil
		},
		Timeout: msDur(d.TimeoutMS),
		Log:     env.Logger(),
	})
	if err != nil {
		return nil, err
	}
	q.Put(name, g)
	return g, nil
}

// framerByName resolves a wire framer; the default is a 2-byte big-endian length
// prefix, the most common ISO-8583-over-TCP framing.
func framerByName(name string) (link.Framer, error) {
	switch name {
	case "", "length2":
		return link.LengthPrefix(2), nil
	case "length4":
		return link.LengthPrefix(4), nil
	default:
		return nil, fmt.Errorf("unknown framer %q (want length2|length4)", name)
	}
}

// hostOf returns the host portion of a host:port address (for TLS server name).
func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

func msDur(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
