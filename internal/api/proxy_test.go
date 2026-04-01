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
	"os"
	"path/filepath"
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

func TestWithProxyTransport_UDSProxy(t *testing.T) {
	target := startTargetServer(t)
	targetURL, _ := url.Parse(target.URL)

	// Use /tmp directly because t.TempDir() paths on macOS could exceed
	// the 108-character limit for unix socket addresses.
	socketPath := filepath.Join("/tmp", fmt.Sprintf("src-cli-test-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { os.Remove(socketPath) })
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	// The UDS proxy path dials the unix socket directly (no CONNECT).
	// Simulate a forwarding proxy that copies bytes to the target.
	var mu sync.Mutex
	var used bool

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			used = true
			mu.Unlock()
			go func() {
				defer conn.Close()
				dest, err := net.DialTimeout("tcp", targetURL.Host, 10*time.Second)
				if err != nil {
					return
				}
				defer dest.Close()
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); io.Copy(dest, conn) }()
				go func() { defer wg.Done(); io.Copy(conn, dest) }()
				wg.Wait()
			}()
		}
	}()

	transport := withProxyTransport(newTestTransport(), nil, socketPath)
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through unix proxy: %v", err)
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
		t.Fatal("unix socket proxy was never used")
	}
}

func TestWithProxyTransport_HTTPProxy(t *testing.T) {
	target := startTargetServer(t)

	var mu sync.Mutex
	var used bool

	proxyURL := startProxy(t, proxyOpts{
		observe: func(r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			used = true
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
}

func TestWithProxyTransport_HTTPSProxy(t *testing.T) {
	target := startTargetServer(t)

	var mu sync.Mutex
	var used bool

	proxyURL := startProxy(t, proxyOpts{
		useTLS: true,
		observe: func(r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			used = true
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
}
