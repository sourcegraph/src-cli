package api

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startTargetServer starts an HTTPS server that responds with "ok".
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

// newTestTransport creates a base transport with TLS verification disabled.
func newTestTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return transport
}

// startCONNECTProxy starts an HTTP CONNECT proxy. If useTLS is true, the proxy
// itself listens over TLS. The returned *atomic.Bool is set to true when the
// proxy handles a request.
func startCONNECTProxy(t *testing.T, useTLS bool) (proxyURL *url.URL, used *atomic.Bool) {
	t.Helper()

	used = &atomic.Bool{}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		used.Store(true)

		if r.Method != http.MethodConnect {
			http.Error(w, "expected CONNECT", http.StatusMethodNotAllowed)
			return
		}

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
	}))

	if useTLS {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)

	pURL, _ := url.Parse(srv.URL)
	return pURL, used
}

// startUDSForwarder creates a unix domain socket that forwards TCP traffic
// to the given target address. Returns the socket path and an *atomic.Bool
// that indicates whether the socket was used.
func startUDSForwarder(t *testing.T, targetAddr string) (socketPath string, used *atomic.Bool) {
	t.Helper()

	socketPath = filepath.Join("/tmp", fmt.Sprintf("src-cli-test-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { os.Remove(socketPath) })

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	used = &atomic.Bool{}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			used.Store(true)
			go func() {
				defer conn.Close()
				dest, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
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

	return socketPath, used
}

func doGET(t *testing.T, transport *http.Transport, targetURL string) {
	t.Helper()
	transport.CloseIdleConnections()
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Errorf("expected body 'ok', got %q", got)
	}
}

func TestWithProxyTransport_HTTPProxy(t *testing.T) {
	target := startTargetServer(t)
	proxyURL, used := startCONNECTProxy(t, false)

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	doGET(t, transport, target.URL)

	if !used.Load() {
		t.Fatal("HTTP proxy was never used")
	}
}

func TestWithProxyTransport_HTTPSProxy(t *testing.T) {
	target := startTargetServer(t)
	proxyURL, used := startCONNECTProxy(t, true)

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	doGET(t, transport, target.URL)

	if !used.Load() {
		t.Fatal("HTTPS proxy was never used")
	}
}

func TestWithProxyTransport_UDSProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets are not supported on Windows")
	}
	target := startTargetServer(t)
	targetURL, _ := url.Parse(target.URL)

	socketPath, used := startUDSForwarder(t, targetURL.Host)

	transport := withProxyTransport(newTestTransport(), nil, socketPath)
	doGET(t, transport, target.URL)

	if !used.Load() {
		t.Fatal("unix socket proxy was never used")
	}
}

func TestWithProxyTransport_UDSClearsSystemProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets are not supported on Windows")
	}
	target := startTargetServer(t)
	targetURL, _ := url.Parse(target.URL)

	// Start a real proxy so we can verify it is NOT used.
	envProxyURL, envProxyUsed := startCONNECTProxy(t, false)
	t.Setenv("HTTPS_PROXY", envProxyURL.String())

	socketPath, udsUsed := startUDSForwarder(t, targetURL.Host)

	// proxyPath should win: Proxy must be set to nil (disabling
	// HTTPS_PROXY) and traffic must go through the unix socket.
	transport := withProxyTransport(newTestTransport(), nil, socketPath)

	if transport.Proxy != nil {
		t.Fatal("expected Proxy to be nil when proxyPath is set")
	}

	doGET(t, transport, target.URL)

	if envProxyUsed.Load() {
		t.Fatal("HTTPS_PROXY was used despite proxyPath being set")
	}
	if !udsUsed.Load() {
		t.Fatal("unix socket proxy was never used")
	}
}

func TestWithProxyTransport_NilProxyPreservesDefault(t *testing.T) {
	transport := newTestTransport()
	originalProxy := transport.Proxy

	result := withProxyTransport(transport, nil, "")

	// When neither proxyURL nor proxyPath is set, the function should
	// not modify the transport at all — the default Proxy function
	// (which reads HTTPS_PROXY/NO_PROXY) should remain.
	if result != transport {
		t.Fatal("expected same transport to be returned")
	}
	// Proxy should remain at whatever the default was (http.ProxyFromEnvironment)
	if originalProxy != nil && result.Proxy == nil {
		t.Fatal("Proxy function was unexpectedly cleared")
	}
}

func TestBuildTransport_WithProxyURL(t *testing.T) {
	target := startTargetServer(t)
	proxyURL, used := startCONNECTProxy(t, false)

	insecure := true
	flags := &Flags{
		insecureSkipVerify: &insecure,
		dump:               boolPtr(false),
		getCurl:            boolPtr(false),
		trace:              boolPtr(false),
		userAgentTelemetry: boolPtr(false),
	}
	opts := ClientOpts{
		ProxyURL: proxyURL,
	}

	transport := buildTransport(opts, flags)
	httpTransport := transport.(*http.Transport)
	doGET(t, httpTransport, target.URL)

	if !used.Load() {
		t.Fatal("proxy was not used via buildTransport")
	}
}

func TestBuildTransport_WithProxyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets are not supported on Windows")
	}
	target := startTargetServer(t)
	targetURL, _ := url.Parse(target.URL)

	socketPath, used := startUDSForwarder(t, targetURL.Host)

	insecure := true
	flags := &Flags{
		insecureSkipVerify: &insecure,
		dump:               boolPtr(false),
		getCurl:            boolPtr(false),
		trace:              boolPtr(false),
		userAgentTelemetry: boolPtr(false),
	}
	opts := ClientOpts{
		ProxyPath: socketPath,
	}

	transport := buildTransport(opts, flags)
	httpTransport := transport.(*http.Transport)
	doGET(t, httpTransport, target.URL)

	if !used.Load() {
		t.Fatal("UDS proxy was not used via buildTransport")
	}
}

func TestBuildTransport_NoProxyPreservesProxyFunc(t *testing.T) {
	// When no ProxyURL/ProxyPath is set, buildTransport should not
	// override the transport's Proxy function. This means Go's default
	// http.ProxyFromEnvironment (reading HTTPS_PROXY/NO_PROXY) remains active.
	//
	// We verify this structurally: the Proxy field should still be set
	// (not nil), meaning environment-based proxy resolution is preserved.
	insecure := true
	flags := &Flags{
		insecureSkipVerify: &insecure,
		dump:               boolPtr(false),
		getCurl:            boolPtr(false),
		trace:              boolPtr(false),
		userAgentTelemetry: boolPtr(false),
	}
	opts := ClientOpts{} // no ProxyURL or ProxyPath

	transport := buildTransport(opts, flags)
	httpTransport := transport.(*http.Transport)

	if httpTransport.Proxy == nil {
		t.Fatal("expected Proxy function to be preserved (for HTTPS_PROXY/NO_PROXY support)")
	}
}

func TestBuildTransport_NOPROXYOverridesHTTPSPROXY(t *testing.T) {
	target := startTargetServer(t)
	targetURL, _ := url.Parse(target.URL)
	host, _, _ := net.SplitHostPort(targetURL.Host)

	proxyURL, used := startCONNECTProxy(t, false)

	// Set HTTPS_PROXY and also NO_PROXY for the target host.
	t.Setenv("HTTPS_PROXY", proxyURL.String())
	t.Setenv("NO_PROXY", host)

	insecure := true
	flags := &Flags{
		insecureSkipVerify: &insecure,
		dump:               boolPtr(false),
		getCurl:            boolPtr(false),
		trace:              boolPtr(false),
		userAgentTelemetry: boolPtr(false),
	}
	opts := ClientOpts{} // no ProxyURL or ProxyPath

	transport := buildTransport(opts, flags)
	httpTransport := transport.(*http.Transport)
	doGET(t, httpTransport, target.URL)

	// The proxy should NOT have been used because NO_PROXY matches the target.
	if used.Load() {
		t.Fatal("proxy was used despite NO_PROXY matching the target host")
	}
}

func TestBuildTransport_ExplicitProxyURLOverridesNOPROXY(t *testing.T) {
	// When ProxyURL is explicitly set (as readConfig does when SRC_PROXY is
	// set), it takes precedence over NO_PROXY because withProxyTransport
	// replaces transport.Proxy with http.ProxyURL, bypassing ProxyFromEnvironment.
	target := startTargetServer(t)
	targetURL, _ := url.Parse(target.URL)
	host, _, _ := net.SplitHostPort(targetURL.Host)

	proxyURL, used := startCONNECTProxy(t, false)

	// NO_PROXY matches the target, but SRC_PROXY (via ProxyURL) should override.
	t.Setenv("NO_PROXY", host)

	insecure := true
	flags := &Flags{
		insecureSkipVerify: &insecure,
		dump:               boolPtr(false),
		getCurl:            boolPtr(false),
		trace:              boolPtr(false),
		userAgentTelemetry: boolPtr(false),
	}
	opts := ClientOpts{
		ProxyURL: proxyURL,
	}

	transport := buildTransport(opts, flags)
	httpTransport := transport.(*http.Transport)
	doGET(t, httpTransport, target.URL)

	if !used.Load() {
		t.Fatal("explicit ProxyURL did not override NO_PROXY")
	}
}

func boolPtr(b bool) *bool { return &b }
