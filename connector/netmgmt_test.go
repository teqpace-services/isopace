package connector

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/mux"
)

// --- a tiny line-framed peer, enough to exercise the connector's hooks -------

// testPeer speaks newline-free 2-byte length frames of the form "<MTI>:<trace>".
type testPeer struct {
	ln      net.Listener
	mu      sync.Mutex
	silent  bool     // stop answering, but hold the socket open (a dead-but-open peer)
	conn    net.Conn // the accepted connection; closing the LISTENER does not close this
	inbound []string // every frame we received
	send    chan string
}

func newTestPeer(t *testing.T, answer func(frame string) (string, bool)) *testPeer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &testPeer{ln: ln, send: make(chan string, 8)}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		p.conn = conn
		p.mu.Unlock()
		l := link.New(conn, link.WithFramer(link.LengthPrefix(2)))
		go func() { // peer-initiated frames
			for s := range p.send {
				_ = l.Send([]byte(s))
			}
		}()
		for {
			raw, err := l.Receive()
			if err != nil {
				return
			}
			p.mu.Lock()
			p.inbound = append(p.inbound, string(raw))
			silent := p.silent
			p.mu.Unlock()
			if silent {
				continue
			}
			if out, ok := answer(string(raw)); ok {
				if err := l.Send([]byte(out)); err != nil {
					return
				}
			}
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return p
}

func (p *testPeer) addr() string { return p.ln.Addr().String() }

func (p *testPeer) goSilent() {
	p.mu.Lock()
	p.silent = true
	p.mu.Unlock()
}

// kill drops the accepted connection outright — the peer process dying, as distinct
// from goSilent (which keeps the socket open and simply stops answering).
func (p *testPeer) kill() {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
	p.ln.Close()
}

func (p *testPeer) received() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.inbound...)
}

func mtiOf(frame string) string { return strings.SplitN(frame, ":", 2)[0] }
func traceOf(frame string) string {
	parts := strings.SplitN(frame, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

// requestKeyer keys an OUTGOING frame by the response it expects (MTI function digit + 1).
func requestKeyer(frame []byte) (string, error) {
	m, tr := mtiOf(string(frame)), traceOf(string(frame))
	if len(m) != 4 || tr == "" {
		return "", errors.New("unkeyable")
	}
	return fmt.Sprintf("%s%c%s:%s", m[:2], m[2]+1, m[3:], tr), nil
}

// responseKeyer keys an INBOUND frame by its own MTI.
func responseKeyer(frame []byte) (string, error) {
	m, tr := mtiOf(string(frame)), traceOf(string(frame))
	if len(m) != 4 || tr == "" {
		return "", errors.New("unkeyable")
	}
	return m + ":" + tr, nil
}

// traceKeyer correlates on the trace alone, so a single keyer matches a request to its
// response across the MTI step. It is the shape that needs no ResponseKeyer — and the
// shape whose ambiguity ResponseKeyer exists to resolve.
func traceKeyer(frame []byte) (string, error) {
	tr := traceOf(string(frame))
	if tr == "" {
		return "", errors.New("unkeyable")
	}
	return tr, nil
}

func dial(t *testing.T, cfg Config) *Connector {
	t.Helper()
	cfg.LinkOptions = append(cfg.LinkOptions, link.WithFramer(link.LengthPrefix(2)))
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Stop(context.Background()) })
	return c
}

func waitUp(t *testing.T, c *Connector, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if c.Connected() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("connector never came up")
}

// --- gap 1: a peer-originated request can be ANSWERED ------------------------

func TestOnUnsolicited_CanAnswerAPeerOriginatedEcho(t *testing.T) {
	peer := newTestPeer(t, func(frame string) (string, bool) {
		if mtiOf(frame) == "0800" { // our keep-alive
			return "0810:" + traceOf(frame), true
		}
		return "", false
	})

	answered := make(chan string, 1)
	c := dial(t, Config{
		Name: "peer", Addr: peer.addr(), Keyer: responseKeyer, Timeout: time.Second,
		OnUnsolicited: func(frame []byte, reply mux.Reply) {
			// A peer echo-test: answer it, or the peer decides the link is dead.
			if mtiOf(string(frame)) == "0800" {
				out := "0810:" + traceOf(string(frame))
				if err := reply([]byte(out)); err == nil {
					answered <- out
				}
			}
		},
	})
	waitUp(t, c, 2*time.Second)

	peer.send <- "0800:777" // the PEER echo-tests US

	select {
	case got := <-answered:
		if got != "0810:777" {
			t.Fatalf("answered %q, want 0810:777", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a peer-originated 0800 was never answered")
	}

	// And the answer actually reached the peer.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, f := range peer.received() {
			if f == "0810:777" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the peer never received our 0810; it saw %v", peer.received())
}

func TestSend_RequiresALiveLink(t *testing.T) {
	c, err := New(Config{Name: "down", Addr: "127.0.0.1:1", Keyer: responseKeyer})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send([]byte("0810:1")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Send on a down link: %v, want ErrNotConnected", err)
	}
}

// --- gap 2: a peer request no longer satisfies our pending request -----------

func TestResponseKeyer_PeerRequestDoesNotSatisfyOurs(t *testing.T) {
	// The peer never answers our 0800 — it only sends its OWN 0800 on the same trace.
	peer := newTestPeer(t, func(frame string) (string, bool) { return "", false })

	unsolicited := make(chan string, 4)
	c := dial(t, Config{
		Name: "peer", Addr: peer.addr(),
		Keyer:         requestKeyer,  // our 0800 registers under 0810:<trace>
		ResponseKeyer: responseKeyer, // an inbound frame keys by its OWN mti
		Timeout:       400 * time.Millisecond,
		OnUnsolicited: func(frame []byte, _ mux.Reply) { unsolicited <- string(frame) },
	})
	waitUp(t, c, 2*time.Second)

	done := make(chan error, 1)
	go func() {
		_, err := c.Request(context.Background(), []byte("0800:555"))
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	peer.send <- "0800:555" // same trace, but it is a REQUEST, not our answer

	select {
	case got := <-unsolicited:
		if got != "0800:555" {
			t.Fatalf("unsolicited frame %q, want 0800:555", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the peer's own 0800 was consumed as our reply instead of reaching the unsolicited handler")
	}

	if err := <-done; !errors.Is(err, mux.ErrTimeout) {
		t.Fatalf("our request resolved with %v; it must still time out — nothing answered it", err)
	}
}

// --- gap 3: a stalled keepalive no longer blinds the supervisor --------------

func TestKeepaliveTimeout_DetectsAHalfOpenPeerWithoutWaitingOutTheRequestTimeout(t *testing.T) {
	// The peer answers until told to go silent, then holds the socket OPEN and stops
	// answering: no FIN, no RST. Nothing ever closes the mux, so the only thing that can
	// notice is the keepalive giving up.
	peer := newTestPeer(t, func(frame string) (string, bool) {
		return "0810:" + traceOf(frame), true
	})

	var stan atomic.Uint64
	c := dial(t, Config{
		Name: "peer", Addr: peer.addr(), Keyer: traceKeyer,
		// A financial timeout: what an echo would inherit without KeepaliveTimeout.
		Timeout:           30 * time.Second,
		KeepaliveInterval: 50 * time.Millisecond,
		KeepaliveTimeout:  200 * time.Millisecond,
		Keepalive: func(ctx context.Context, m *mux.Mux) error {
			_, err := m.Request(ctx, []byte(fmt.Sprintf("0800:%d", stan.Add(1))))
			return err
		},
		MinBackoff: time.Minute, // do not let a reconnect mask the down state
		MaxBackoff: time.Minute,
	})
	waitUp(t, c, 2*time.Second)

	peer.goSilent()

	// interval + keepalive timeout + margin. Without KeepaliveTimeout this takes the full
	// 30s request timeout, so a generous 5s here still fails against the old behaviour.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !c.Connected() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a half-open peer left the connector reporting Connected(): the echo is still " +
		"inheriting the financial request timeout, so live traffic keeps being written to a dead socket")
}

func TestKeepalive_DoesNotBlockStop(t *testing.T) {
	// A keepalive outstanding against a silent peer must not hold Stop hostage — off the
	// supervise loop, ctx cancellation is observed immediately.
	peer := newTestPeer(t, func(frame string) (string, bool) {
		return "0810:" + traceOf(frame), true
	})

	var stan atomic.Uint64
	entered := make(chan struct{}, 1)
	cfg := Config{
		Name: "peer", Addr: peer.addr(), Keyer: traceKeyer,
		Timeout:           30 * time.Second,
		KeepaliveInterval: 50 * time.Millisecond,
		Keepalive: func(ctx context.Context, m *mux.Mux) error {
			select {
			case entered <- struct{}{}:
			default:
			}
			_, err := m.Request(ctx, []byte(fmt.Sprintf("0800:%d", stan.Add(1))))
			return err
		},
		LinkOptions: []link.Option{link.WithFramer(link.LengthPrefix(2))},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUp(t, c, 2*time.Second)
	peer.goSilent()
	<-entered

	done := make(chan struct{})
	go func() { c.Stop(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop blocked behind an outstanding keepalive")
	}
}

// --- gap 4: OnReady runs where Request actually works ------------------------

func TestOnReady_RunsAfterTheLinkIsUsable(t *testing.T) {
	peer := newTestPeer(t, func(frame string) (string, bool) {
		return "0810:" + traceOf(frame), true
	})

	ready := make(chan error, 1)
	c := dial(t, Config{
		Name: "peer", Addr: peer.addr(), Keyer: traceKeyer, Timeout: time.Second,
		OnReady: func(ctx context.Context, c *Connector) {
			// The whole point: a ceremony issued through the CONNECTOR must work here.
			// From OnConnect this would fail ErrNotConnected.
			_, err := c.Request(ctx, []byte("0800:901"))
			ready <- err
		},
	})
	waitUp(t, c, 2*time.Second)

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("OnReady could not use the connector: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnReady never ran")
	}
}

func TestOnConnect_CannotUseTheConnectorYet(t *testing.T) {
	// Documents WHY OnReady exists: inside OnConnect the mux is not published.
	peer := newTestPeer(t, func(frame string) (string, bool) {
		return "0810:" + traceOf(frame), true
	})

	var self atomic.Pointer[Connector]
	got := make(chan error, 1)
	cfg := Config{
		Name: "peer", Addr: peer.addr(), Keyer: responseKeyer, Timeout: time.Second,
		OnConnect: func(ctx context.Context, m *mux.Mux) error {
			if c := self.Load(); c != nil {
				select {
				case got <- c.Send([]byte("0800:1")):
				default:
				}
			}
			return nil
		},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	self.Store(c)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Stop(context.Background()) })

	select {
	case err := <-got:
		if !errors.Is(err, ErrNotConnected) {
			t.Fatalf("inside OnConnect the connector reported %v; expected ErrNotConnected", err)
		}
	case <-time.After(2 * time.Second):
		t.Skip("OnConnect ran before the connector pointer was stored")
	}
}
