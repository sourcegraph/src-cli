package api

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type proxyOpts struct {
	useTLS   bool
	username string
	password string
	observe  func(*http.Request)
}

// startProxy starts an HTTP or HTTPS CONNECT proxy on a random port.
// If opts.observe is set, it is called for each CONNECT request.
// If opts.username is set, Proxy-Authorization is required.
func startProxy(t *testing.T, opts proxyOpts) *url.URL {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.observe != nil {
			opts.observe(r)
		}

		if r.Method != http.MethodConnect {
			http.Error(w, "expected CONNECT", http.StatusMethodNotAllowed)
			return
		}

		if opts.username != "" {
			wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(opts.username+":"+opts.password))
			if r.Header.Get("Proxy-Authorization") != wantAuth {
				http.Error(w, "proxy auth required", http.StatusProxyAuthRequired)
				return
			}
		}

		serveTunnel(w, r)
	}))

	if opts.useTLS {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)

	pURL, _ := url.Parse(srv.URL)
	if opts.username != "" {
		pURL.User = url.UserPassword(opts.username, opts.password)
	}
	return pURL
}

// serveTunnel implements the CONNECT tunnel: dials the target, hijacks the
// client connection, and copies bytes bidirectionally.
func serveTunnel(w http.ResponseWriter, r *http.Request) {
	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer destConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var once sync.Once
	closeBoth := func() {
		clientConn.Close()
		destConn.Close()
	}
	defer once.Do(closeBoth)

	wg.Add(2)
	// Read from bufrw (not clientConn) so any bytes already buffered
	// by the server's bufio.Reader are forwarded to the destination.
	go func() {
		defer wg.Done()
		io.Copy(destConn, bufrw)
		once.Do(closeBoth)
	}()
	go func() {
		defer wg.Done()
		io.Copy(clientConn, destConn)
		once.Do(closeBoth)
	}()
	wg.Wait()
}

// newTestTransport creates a base transport suitable for proxy tests.
func newTestTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return transport
}

// startTargetServer starts an HTTPS server (with HTTP/2 enabled) that
// responds with "ok" to GET /.
func startTargetServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func TestWithProxyTransport_HTTPProxy(t *testing.T) {
	target := startTargetServer(t)

	var mu sync.Mutex
	var used bool
	var proto string

	proxyURL := startProxy(t, proxyOpts{
		observe: func(r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			used = true
			proto = r.Proto
		},
	})

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through http proxy: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Errorf("expected body 'ok', got %q", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if !used {
		t.Fatal("proxy handler was never invoked")
	}
	if proto != "HTTP/1.1" {
		t.Errorf("expected proxy to see HTTP/1.1 CONNECT, got %s", proto)
	}
}

func TestWithProxyTransport_HTTPSProxy(t *testing.T) {
	target := startTargetServer(t)

	var mu sync.Mutex
	var used bool
	var proto string

	proxyURL := startProxy(t, proxyOpts{
		useTLS: true,
		observe: func(r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			used = true
			proto = r.Proto
		},
	})

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through https proxy: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Errorf("expected body 'ok', got %q", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if !used {
		t.Fatal("proxy handler was never invoked")
	}
	if proto != "HTTP/1.1" {
		t.Errorf("expected proxy to see HTTP/1.1 CONNECT, got %s", proto)
	}
}

func TestWithProxyTransport_ProxyAuth(t *testing.T) {
	target := startTargetServer(t)

	t.Run("http proxy with auth", func(t *testing.T) {
		proxyURL := startProxy(t, proxyOpts{username: "user", password: "pass"})
		transport := withProxyTransport(newTestTransport(), proxyURL, "")
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatalf("GET through authenticated http proxy: %v", err)
		}
		defer resp.Body.Close()
		if _, err := io.ReadAll(resp.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("https proxy with auth", func(t *testing.T) {
		proxyURL := startProxy(t, proxyOpts{useTLS: true, username: "user", password: "s3cret"})
		transport := withProxyTransport(newTestTransport(), proxyURL, "")
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatalf("GET through authenticated https proxy: %v", err)
		}
		defer resp.Body.Close()
		if _, err := io.ReadAll(resp.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestWithProxyTransport_HTTPSProxy_HTTP2ToOrigin(t *testing.T) {
	// Verify that when tunneling through an HTTPS proxy, the connection to
	// the origin target still negotiates HTTP/2 (not downgraded to HTTP/1.1).
	target := startTargetServer(t)
	proxyURL := startProxy(t, proxyOpts{useTLS: true})

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through https proxy: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.ProtoMajor != 2 {
		t.Errorf("expected HTTP/2 to origin, got %s", resp.Proto)
	}
}

func TestWithProxyTransport_HandshakeFailureClosesConn(t *testing.T) {
	// Verify that when the TLS handshake to the origin fails, the underlying
	// tunnel connection is closed (regression test for tlsConn.Close on error).
	//
	// A plain TCP listener acts as the target. The proxy CONNECT succeeds
	// (TCP-level), but the subsequent TLS handshake fails because the target
	// is not a TLS server. If handshakeTLS properly closes tlsConn on failure,
	// the tunnel tears down and the target sees the connection close.
	connClosed := make(chan struct{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send non-TLS bytes so the client handshake fails immediately
		// rather than waiting for a timeout.
		conn.Write([]byte("not-tls\n"))
		// Drain until the remote side closes the tunnel.
		io.Copy(io.Discard, conn)
		close(connClosed)
	}()

	proxyURL := startProxy(t, proxyOpts{useTLS: true})
	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	_, err = client.Get("https://" + ln.Addr().String())
	if err == nil {
		t.Fatal("expected TLS handshake error, got nil")
	}

	select {
	case <-connClosed:
		// Connection was properly cleaned up.
	case <-time.After(5 * time.Second):
		t.Fatal("connection was not closed after TLS handshake failure")
	}
}

func TestWithProxyTransport_ProxyRejectsConnect(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{"407 proxy auth required", http.StatusProxyAuthRequired, "proxy auth required", "Proxy Authentication Required"},
		{"403 forbidden", http.StatusForbidden, "access denied by policy", "Forbidden"},
		{"502 bad gateway", http.StatusBadGateway, "upstream unreachable", "Bad Gateway"},
	}

	// Use a local target so we never depend on external DNS.
	target := startTargetServer(t)

	for _, tt := range tests {
		t.Run("http proxy/"+tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tt.body, tt.statusCode)
			}))
			t.Cleanup(srv.Close)

			proxyURL, _ := url.Parse(srv.URL)
			transport := withProxyTransport(newTestTransport(), proxyURL, "")
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

			_, err := client.Get(target.URL)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got: %v", tt.wantErr, err)
			}
		})

		t.Run("https proxy/"+tt.name, func(t *testing.T) {
			// The HTTPS proxy path uses a custom dialer with its own error
			// formatting that includes the status, body, and redacted proxy URL.
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tt.body, tt.statusCode)
			}))
			srv.StartTLS()
			t.Cleanup(srv.Close)

			proxyURL, _ := url.Parse(srv.URL)
			transport := withProxyTransport(newTestTransport(), proxyURL, "")
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

			_, err := client.Get(target.URL)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", tt.statusCode)) {
				t.Errorf("error should contain status code %d, got: %v", tt.statusCode, err)
			}
			if !strings.Contains(err.Error(), tt.body) {
				t.Errorf("error should contain body %q, got: %v", tt.body, err)
			}
		})
	}
}

func TestProxyDialAddr(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"https with port", "https://proxy.example.com:8443", "proxy.example.com:8443"},
		{"https without port", "https://proxy.example.com", "proxy.example.com:443"},
		{"http with port", "http://proxy.example.com:8080", "proxy.example.com:8080"},
		{"http without port", "http://proxy.example.com", "proxy.example.com:80"},
		{"ipv4 with port", "http://192.168.1.100:3128", "192.168.1.100:3128"},
		{"ipv4 without port https", "https://10.0.0.1", "10.0.0.1:443"},
		{"ipv4 without port http", "http://172.16.0.5", "172.16.0.5:80"},
		{"ipv6 with port", "http://[::1]:8080", "[::1]:8080"},
		{"ipv6 without port https", "https://[2001:db8::1]", "[2001:db8::1]:443"},
		{"ipv6 without port http", "http://[fe80::1]", "[fe80::1]:80"},
		{"localhost with port", "http://localhost:9090", "localhost:9090"},
		{"localhost without port https", "https://localhost", "localhost:443"},
		{"localhost without port http", "http://localhost", "localhost:80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.ParseRequestURI(tt.url)
			if err != nil {
				t.Fatalf("parse URL: %v", err)
			}
			got := proxyDialAddr(u)
			if got != tt.want {
				t.Errorf("proxyHostPort(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
