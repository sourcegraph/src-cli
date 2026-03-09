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
	"testing"
	"time"
)

// startProxy starts an HTTP or HTTPS CONNECT proxy on a random port.
// It returns the proxy URL and a channel that receives the protocol observed by
// the proxy handler for each CONNECT request.
func startProxy(t *testing.T, useTLS bool) (proxyURL *url.URL, obsCh <-chan string) {
	t.Helper()

	ch := make(chan string, 10)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case ch <- r.Proto:
		default:
		}

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
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer clientConn.Close()

		done := make(chan struct{}, 2)
		go func() { io.Copy(destConn, clientConn); done <- struct{}{} }()
		go func() { io.Copy(clientConn, destConn); done <- struct{}{} }()
		<-done
		// Close both sides so the remaining goroutine unblocks.
		clientConn.Close()
		destConn.Close()
		<-done
	}))

	if useTLS {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)

	pURL, _ := url.Parse(srv.URL)
	return pURL, ch
}

// startProxyWithAuth is like startProxy but requires
// Proxy-Authorization with the given username and password.
func startProxyWithAuth(t *testing.T, useTLS bool, wantUser, wantPass string) (proxyURL *url.URL) {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "expected CONNECT", http.StatusMethodNotAllowed)
			return
		}

		authHeader := r.Header.Get("Proxy-Authorization")
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(wantUser+":"+wantPass))
		if authHeader != wantAuth {
			http.Error(w, "proxy auth required", http.StatusProxyAuthRequired)
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
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer clientConn.Close()

		done := make(chan struct{}, 2)
		go func() { io.Copy(destConn, clientConn); done <- struct{}{} }()
		go func() { io.Copy(clientConn, destConn); done <- struct{}{} }()
		<-done
		clientConn.Close()
		destConn.Close()
		<-done
	}))

	if useTLS {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)

	pURL, _ := url.Parse(srv.URL)
	pURL.User = url.UserPassword(wantUser, wantPass)
	return pURL
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
	proxyURL, obsCh := startProxy(t, false)

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through http proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Errorf("expected body 'ok', got %q", got)
	}

	select {
	case proto := <-obsCh:
		if proto != "HTTP/1.1" {
			t.Errorf("expected proxy to see HTTP/1.1 CONNECT, got %s", proto)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy handler was never invoked")
	}
}

func TestWithProxyTransport_HTTPSProxy(t *testing.T) {
	target := startTargetServer(t)
	proxyURL, obsCh := startProxy(t, true)

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through https proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Errorf("expected body 'ok', got %q", got)
	}

	select {
	case proto := <-obsCh:
		if proto != "HTTP/1.1" {
			t.Errorf("expected proxy to see HTTP/1.1 CONNECT, got %s", proto)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy handler was never invoked")
	}
}

func TestWithProxyTransport_ProxyAuth(t *testing.T) {
	target := startTargetServer(t)

	t.Run("http proxy with auth", func(t *testing.T) {
		proxyURL := startProxyWithAuth(t, false, "user", "pass")
		transport := withProxyTransport(newTestTransport(), proxyURL, "")
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatalf("GET through authenticated http proxy: %v", err)
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("https proxy with auth", func(t *testing.T) {
		proxyURL := startProxyWithAuth(t, true, "user", "s3cret")
		transport := withProxyTransport(newTestTransport(), proxyURL, "")
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatalf("GET through authenticated https proxy: %v", err)
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestWithProxyTransport_HTTPSProxy_HTTP2ToOrigin(t *testing.T) {
	// Verify that when tunneling through an HTTPS proxy, the connection to
	// the origin target still negotiates HTTP/2 (not downgraded to HTTP/1.1).
	target := startTargetServer(t)
	proxyURL, _ := startProxy(t, true)

	transport := withProxyTransport(newTestTransport(), proxyURL, "")
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through https proxy: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.Proto != "HTTP/2.0" {
		t.Errorf("expected HTTP/2.0 to origin, got %s", resp.Proto)
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start a proxy that always rejects CONNECT with the given status.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tt.body, tt.statusCode)
			}))
			t.Cleanup(srv.Close)

			proxyURL, _ := url.Parse(srv.URL)
			transport := withProxyTransport(newTestTransport(), proxyURL, "")
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

			_, err := client.Get("https://example.com")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got: %v", tt.wantErr, err)
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
