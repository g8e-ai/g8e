// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration || e2e

package fixtures

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/protocol"
)

// WireListener names the witness listener an exchange arrived on: the plain
// listener stands in for the Gateway HTTP port, the TLS listener for the
// Gateway mTLS port.
type WireListener string

const (
	WireListenerPlain WireListener = "plain"
	WireListenerTLS   WireListener = "tls"
)

// WireExchange is one request an Operator sent toward the Gateway. Upgrade
// marks a websocket handshake; every other exchange is an HTTP request.
// ClientCertSerial is the decimal serial of the client certificate presented
// on the TLS listener, empty when none was presented.
type WireExchange struct {
	Listener         WireListener
	Method           string
	Path             string
	Upgrade          bool
	ClientCertSerial string
	At               time.Time
}

func (e WireExchange) String() string {
	kind := "http"
	if e.Upgrade {
		kind = "websocket"
	}
	return fmt.Sprintf("%s %s %s %s serial=%q", e.Listener, kind, e.Method, e.Path, e.ClientCertSerial)
}

// RefusedDial is a connection the witness refused while partitioned.
type RefusedDial struct {
	Listener WireListener
	At       time.Time
}

// OperatorWireWitness is the network between Operators under test and a real
// Gateway. Operators dial the witness instead of the Gateway; it records every
// exchange, forwards it unchanged, and can sever, partition, or cut the link to
// model a disconnected, intermittent, or limited (DDIL) network. It holds no
// policy: the Gateway behind it is the enforcement point under test.
//
// TLS is terminated with the Gateway's own serving certificate, so an Operator
// verifies the witness exactly as it verifies the Gateway. Requests are
// re-originated to the Gateway with the client certificate the Operator
// presented, read from the Operator's in-memory ClientIdentity.
type OperatorWireWitness struct {
	gateway   *GatewayFixture
	rootPool  *x509.CertPool
	plainAddr string
	tlsAddr   string
	servers   []*http.Server

	exchanges *eventLog[WireExchange]
	refused   *eventLog[RefusedDial]
	faults    *eventLog[error]

	mu          sync.Mutex
	partitioned bool
	conns       map[*witnessConn]struct{}
	identities  []*certs.ClientIdentity
	transports  map[string]*http.Transport
	cutArmed    bool
	cutFired    chan struct{}
	cutOnce     sync.Once

	relays sync.WaitGroup
}

// NewOperatorWireWitness starts a witness in front of f. Its listeners close,
// and its relays are joined, when the test ends.
func NewOperatorWireWitness(t *testing.T, f *GatewayFixture) *OperatorWireWitness {
	t.Helper()

	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(testutil.ReadRootCA(t, f.PKIDir)), "wire witness: parse root CA")

	w := &OperatorWireWitness{
		gateway:    f,
		rootPool:   rootPool,
		exchanges:  newEventLog[WireExchange](),
		refused:    newEventLog[RefusedDial](),
		faults:     newEventLog[error](),
		conns:      make(map[*witnessConn]struct{}),
		transports: make(map[string]*http.Transport),
		cutFired:   make(chan struct{}),
	}

	plainLn, err := net.Listen("tcp", net.JoinHostPort(constants.LocalhostIP, "0"))
	require.NoError(t, err)
	tlsLn, err := net.Listen("tcp", net.JoinHostPort(constants.LocalhostIP, "0"))
	require.NoError(t, err)
	w.plainAddr = plainLn.Addr().String()
	w.tlsAddr = tlsLn.Addr().String()

	serverTLS := f.Service.GetPKI().TLSConfig()
	// The Gateway is the enforcement point; the witness only records which
	// certificate was presented, so it accepts a handshake without one.
	serverTLS.ClientAuth = tls.VerifyClientCertIfGiven
	serverTLS.NextProtos = []string{"http/1.1"}

	plainSrv := &http.Server{Handler: w.handler(WireListenerPlain), ReadHeaderTimeout: 10 * time.Second}
	tlsSrv := &http.Server{Handler: w.handler(WireListenerTLS), ReadHeaderTimeout: 10 * time.Second}
	w.servers = []*http.Server{plainSrv, tlsSrv}

	served := make(chan struct{}, 2)
	go func() {
		_ = plainSrv.Serve(&witnessListener{Listener: plainLn, witness: w, name: WireListenerPlain})
		served <- struct{}{}
	}()
	go func() {
		_ = tlsSrv.Serve(tls.NewListener(&witnessListener{Listener: tlsLn, witness: w, name: WireListenerTLS}, serverTLS))
		served <- struct{}{}
	}()

	t.Cleanup(func() {
		for _, srv := range w.servers {
			_ = srv.Close()
		}
		w.Sever()
		<-served
		<-served
		w.relays.Wait()
		w.mu.Lock()
		for _, transport := range w.transports {
			transport.CloseIdleConnections()
		}
		w.mu.Unlock()
		for _, fault := range w.faults.snapshot() {
			t.Errorf("wire witness: %v", fault)
		}
	})
	return w
}

// PlainPort is the port that stands in for the Gateway HTTP port.
func (w *OperatorWireWitness) PlainPort() int { return addrPort(w.plainAddr) }

// TLSPort is the port that stands in for the Gateway mTLS port.
func (w *OperatorWireWitness) TLSPort() int { return addrPort(w.tlsAddr) }

// Present registers the in-memory identity of an Operator dialing the witness,
// so the witness can re-originate that Operator's TLS connections with the
// certificate it presented.
func (w *OperatorWireWitness) Present(identity *certs.ClientIdentity) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.identities = append(w.identities, identity)
}

// Exchanges returns every exchange recorded so far, in arrival order.
func (w *OperatorWireWitness) Exchanges() []WireExchange {
	return w.exchanges.snapshot()
}

// HTTPRequests returns every recorded exchange that is not a websocket
// handshake.
func (w *OperatorWireWitness) HTTPRequests() []WireExchange {
	var requests []WireExchange
	for _, exchange := range w.exchanges.snapshot() {
		if !exchange.Upgrade {
			requests = append(requests, exchange)
		}
	}
	return requests
}

// Mark returns a position in the exchange record; AwaitExchangeAfter only
// considers exchanges recorded at or after it.
func (w *OperatorWireWitness) Mark() int {
	return w.exchanges.len()
}

// AwaitExchangeAfter blocks until an exchange recorded at or after mark
// satisfies match.
func (w *OperatorWireWitness) AwaitExchangeAfter(ctx context.Context, mark int, match func(WireExchange) bool) (WireExchange, error) {
	exchange, _, err := w.exchanges.await(ctx, mark, match)
	if err != nil {
		return WireExchange{}, fmt.Errorf("wire witness: await exchange: %w (recorded: %v)", err, w.exchanges.snapshot())
	}
	return exchange, nil
}

// AwaitRefusedDials blocks until at least n connections were refused while
// partitioned.
func (w *OperatorWireWitness) AwaitRefusedDials(ctx context.Context, n int) error {
	_, err := w.refused.awaitAll(ctx, func(refused []RefusedDial) bool { return len(refused) >= n })
	if err != nil {
		return fmt.Errorf("wire witness: await %d refused dials: %w", n, err)
	}
	return nil
}

// Sever drops every open connection, as a network outage would. New
// connections are admitted. It returns the number of connections dropped.
func (w *OperatorWireWitness) Sever() int {
	w.mu.Lock()
	conns := make([]*witnessConn, 0, len(w.conns))
	for conn := range w.conns {
		conns = append(conns, conn)
	}
	transports := make([]*http.Transport, 0, len(w.transports))
	for _, transport := range w.transports {
		transports = append(transports, transport)
	}
	w.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
	for _, transport := range transports {
		transport.CloseIdleConnections()
	}
	return len(conns)
}

// Partition severs the link and refuses every new connection until Heal.
func (w *OperatorWireWitness) Partition() {
	w.mu.Lock()
	w.partitioned = true
	w.mu.Unlock()
	w.Sever()
}

// Heal ends a partition.
func (w *OperatorWireWitness) Heal() {
	w.mu.Lock()
	w.partitioned = false
	w.mu.Unlock()
}

// ArmIdentityCut arms a one-shot cut: the first Gateway-to-Operator payload
// that carries an issued Operator certificate is dropped and both ends of its
// connection are closed before the Operator receives it, so the Gateway has
// committed an identity the Operator never saw.
func (w *OperatorWireWitness) ArmIdentityCut() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cutArmed = true
}

// AwaitIdentityCut blocks until the armed cut has fired.
func (w *OperatorWireWitness) AwaitIdentityCut(ctx context.Context) error {
	select {
	case <-w.cutFired:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wire witness: identity cut never fired: %w", ctx.Err())
	}
}

func (w *OperatorWireWitness) cutPending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.cutArmed {
		return false
	}
	select {
	case <-w.cutFired:
		return false
	default:
		return true
	}
}

// takeCut fires the armed cut and reports whether this caller fired it.
func (w *OperatorWireWitness) takeCut() bool {
	if !w.cutPending() {
		return false
	}
	fired := false
	w.cutOnce.Do(func() {
		close(w.cutFired)
		fired = true
	})
	return fired
}

type clientSerialKey struct{}

func (w *OperatorWireWitness) handler(listener WireListener) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(w.gatewayURL(listener))
		},
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			serial, _ := req.Context().Value(clientSerialKey{}).(string)
			transport, err := w.transport(listener, serial)
			if err != nil {
				return nil, err
			}
			return transport.RoundTrip(req)
		}),
		ModifyResponse: w.inspectResponse,
		ErrorHandler: func(rw http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == nil {
				w.faults.append(fmt.Errorf("forward %s %s: %w", r.Method, r.URL.Path, err))
			}
			rw.WriteHeader(http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		exchange := WireExchange{
			Listener:         listener,
			Method:           r.Method,
			Path:             r.URL.Path,
			Upgrade:          isUpgrade(r),
			ClientCertSerial: presentedSerial(r.TLS),
			At:               time.Now(),
		}
		w.exchanges.append(exchange)
		r = r.WithContext(context.WithValue(r.Context(), clientSerialKey{}, exchange.ClientCertSerial))
		if exchange.Upgrade {
			w.relayUpgrade(rw, r, listener, exchange.ClientCertSerial)
			return
		}
		proxy.ServeHTTP(rw, r)
	})
}

// inspectResponse aborts the response when the armed identity cut matches it.
// Aborting the handler closes the Operator's connection without a response.
func (w *OperatorWireWitness) inspectResponse(resp *http.Response) error {
	if !w.cutPending() {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("wire witness: read response: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	inspected := body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		if zr, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
			if plain, err := io.ReadAll(zr); err == nil {
				inspected = plain
			}
		}
	}
	if carriesOperatorIdentity(inspected) && w.takeCut() {
		panic(http.ErrAbortHandler)
	}
	return nil
}

// relayUpgrade forwards a websocket handshake to the Gateway and relays the
// connection in both directions until either end closes.
func (w *OperatorWireWitness) relayUpgrade(rw http.ResponseWriter, r *http.Request, listener WireListener, serial string) {
	backend, err := w.dialGateway(r.Context(), listener, serial)
	if err != nil {
		w.faults.append(fmt.Errorf("dial gateway for %s: %w", r.URL.Path, err))
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	hijacker, ok := rw.(http.Hijacker)
	if !ok {
		_ = backend.Close()
		w.faults.append(fmt.Errorf("upgrade %s: response writer cannot hijack", r.URL.Path))
		http.Error(rw, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = backend.Close()
		w.faults.append(fmt.Errorf("upgrade %s: hijack: %w", r.URL.Path, err))
		return
	}
	if err := r.Write(backend); err != nil {
		_ = client.Close()
		_ = backend.Close()
		return
	}

	w.relays.Add(2)
	go func() {
		defer w.relays.Done()
		defer backend.Close()
		_, _ = io.Copy(backend, buffered.Reader)
	}()
	go func() {
		defer w.relays.Done()
		defer client.Close()
		w.pumpToOperator(client, backend)
	}()
}

// pumpToOperator copies Gateway-to-Operator bytes, firing the armed identity
// cut before forwarding the chunk that completes an issued certificate.
func (w *OperatorWireWitness) pumpToOperator(client, backend net.Conn) {
	const window = 1 << 20
	var seen []byte
	buf := make([]byte, 32<<10)
	for {
		n, err := backend.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if w.cutPending() {
				seen = append(seen, chunk...)
				if len(seen) > window {
					seen = seen[len(seen)-window:]
				}
				if carriesOperatorIdentity(seen) && w.takeCut() {
					_ = client.Close()
					_ = backend.Close()
					return
				}
			}
			if _, werr := client.Write(chunk); werr != nil {
				_ = backend.Close()
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (w *OperatorWireWitness) gatewayURL(listener WireListener) *url.URL {
	if listener == WireListenerTLS {
		return &url.URL{Scheme: "https", Host: net.JoinHostPort(constants.LocalhostIP, strconv.Itoa(w.gateway.Service.GetHTTPSPort()))}
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(constants.LocalhostIP, strconv.Itoa(w.gateway.Service.GetHTTPPort()))}
}

func (w *OperatorWireWitness) dialGateway(ctx context.Context, listener WireListener, serial string) (net.Conn, error) {
	addr := w.gatewayURL(listener).Host
	var conn net.Conn
	var err error
	if listener == WireListenerTLS {
		cfg, cfgErr := w.clientTLS(serial)
		if cfgErr != nil {
			return nil, cfgErr
		}
		conn, err = (&tls.Dialer{Config: cfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	tracked := &witnessConn{Conn: conn, witness: w}
	w.mu.Lock()
	w.conns[tracked] = struct{}{}
	w.mu.Unlock()
	return tracked, nil
}

// transport returns the Gateway transport for listener, re-originating TLS
// with the certificate whose serial the Operator presented.
func (w *OperatorWireWitness) transport(listener WireListener, serial string) (*http.Transport, error) {
	key := string(listener) + "/" + serial
	w.mu.Lock()
	transport, ok := w.transports[key]
	w.mu.Unlock()
	if ok {
		return transport, nil
	}
	transport = &http.Transport{}
	if listener == WireListenerTLS {
		cfg, err := w.clientTLS(serial)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = cfg
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.transports[key]; ok {
		return existing, nil
	}
	w.transports[key] = transport
	return transport, nil
}

func (w *OperatorWireWitness) clientTLS(serial string) (*tls.Config, error) {
	cfg := &tls.Config{
		RootCAs:          w.rootPool,
		ServerName:       constants.GatewayInternalHostname,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: certs.FIPSCurvePreferences(),
		NextProtos:       []string{"http/1.1"},
	}
	if serial == "" {
		return cfg, nil
	}
	cert, err := w.presentedCertificate(serial)
	if err != nil {
		return nil, err
	}
	cfg.Certificates = []tls.Certificate{cert}
	return cfg, nil
}

// presentedCertificate finds the registered in-memory identity whose current
// certificate has serial.
func (w *OperatorWireWitness) presentedCertificate(serial string) (tls.Certificate, error) {
	w.mu.Lock()
	identities := append([]*certs.ClientIdentity(nil), w.identities...)
	w.mu.Unlock()
	for _, identity := range identities {
		cert, ok := identity.GetCertificate()
		if !ok {
			continue
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err == nil && leaf.SerialNumber.String() == serial {
			return cert, nil
		}
	}
	return tls.Certificate{}, fmt.Errorf("no registered Operator identity holds certificate serial %s", serial)
}

// admit reports whether a new connection may proceed, tracking it if so.
func (w *OperatorWireWitness) admit(conn *witnessConn, listener WireListener) bool {
	w.mu.Lock()
	partitioned := w.partitioned
	if !partitioned {
		w.conns[conn] = struct{}{}
	}
	w.mu.Unlock()
	if partitioned {
		w.refused.append(RefusedDial{Listener: listener, At: time.Now()})
	}
	return !partitioned
}

func (w *OperatorWireWitness) forget(conn *witnessConn) {
	w.mu.Lock()
	delete(w.conns, conn)
	w.mu.Unlock()
}

// witnessListener admits connections through the witness so Sever and
// Partition can reach them.
type witnessListener struct {
	net.Listener
	witness *OperatorWireWitness
	name    WireListener
}

func (l *witnessListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		tracked := &witnessConn{Conn: conn, witness: l.witness}
		if l.witness.admit(tracked, l.name) {
			return tracked, nil
		}
		_ = conn.Close()
	}
}

// witnessConn is a connection the witness can drop.
type witnessConn struct {
	net.Conn
	witness *OperatorWireWitness
	once    sync.Once
}

func (c *witnessConn) Close() error {
	var err error
	c.once.Do(func() {
		c.witness.forget(c)
		err = c.Conn.Close()
	})
	return err
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func isUpgrade(r *http.Request) bool {
	return r.Header.Get("Upgrade") != ""
}

func presentedSerial(state *tls.ConnectionState) string {
	if state == nil || len(state.PeerCertificates) == 0 {
		return ""
	}
	return state.PeerCertificates[0].SerialNumber.String()
}

func addrPort(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return n
}

// OperatorWorkloadOf returns the Operator identity cert carries in its SPIFFE
// URI SAN. It returns false for a CA or any non-Operator certificate.
func OperatorWorkloadOf(cert *x509.Certificate) (protocol.OperatorWorkload, bool) {
	if cert == nil || cert.IsCA {
		return protocol.OperatorWorkload{}, false
	}
	wid := protocol.NewWorkloadIdentity()
	for _, uri := range cert.URIs {
		if workload, ok := wid.ExtractOperatorIdentity(uri.String()); ok {
			return workload, true
		}
	}
	return protocol.OperatorWorkload{}, false
}

// carriesOperatorIdentity reports whether data holds a PEM certificate issued
// to an Operator workload, raw or JSON-escaped.
func carriesOperatorIdentity(data []byte) bool {
	const begin = "-----BEGIN CERTIFICATE-----"
	text := bytes.ReplaceAll(data, []byte(`\n`), []byte("\n"))
	for {
		start := bytes.Index(text, []byte(begin))
		if start < 0 {
			return false
		}
		block, rest := pem.Decode(text[start:])
		if block == nil {
			text = text[start+len(begin):]
			continue
		}
		text = rest
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if _, ok := OperatorWorkloadOf(cert); ok {
			return true
		}
	}
}
