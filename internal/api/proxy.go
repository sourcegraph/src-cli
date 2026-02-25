package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

type connWithBufferedReader struct {
	net.Conn
	r *bufio.Reader
}

func (c *connWithBufferedReader) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// proxyDialAddr returns proxyURL.Host with a default port appended if one is
// not already present (443 for https, 80 for http).
func proxyDialAddr(proxyURL *url.URL) string {
	addr := proxyURL.Host
	if _, _, err := net.SplitHostPort(addr); err != nil {
		if proxyURL.Scheme == "https" {
			return net.JoinHostPort(addr, "443")
		}
		return net.JoinHostPort(addr, "80")
	}
	return addr
}

// withProxyTransport modifies the given transport to handle proxying of unix, socks5 and http connections.
//
// Note: baseTransport is considered to be a clone created with transport.Clone()
//
// - If proxyPath is not empty, a unix socket proxy is created.
// - Otherwise, proxyURL is used to determine if we should proxy socks5 / http connections
func withProxyTransport(baseTransport *http.Transport, proxyURL *url.URL, proxyPath string) *http.Transport {
	handshakeTLS := func(ctx context.Context, conn net.Conn, addr string) (net.Conn, error) {
		// Extract the hostname (without the port) for TLS SNI
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		cfg := baseTransport.TLSClientConfig.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = host
		}
		// Preserve HTTP/2 negotiation to the origin when ForceAttemptHTTP2
		// is enabled. Without this, the manual TLS handshake would not
		// advertise h2 via ALPN, silently forcing HTTP/1.1.
		if baseTransport.ForceAttemptHTTP2 && len(cfg.NextProtos) == 0 {
			cfg.NextProtos = []string{"h2", "http/1.1"}
		}
		tlsConn := tls.Client(conn, cfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		return tlsConn, nil
	}

	if proxyPath != "" {
		dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", proxyPath)
		}
		dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return handshakeTLS(ctx, conn, addr)
		}
		baseTransport.DialContext = dial
		baseTransport.DialTLSContext = dialTLS
		// clear out any system proxy settings
		baseTransport.Proxy = nil
	} else if proxyURL != nil {
		switch proxyURL.Scheme {
		case "socks5", "socks5h":
			// SOCKS proxies work out of the box - no need to manually dial
			baseTransport.Proxy = http.ProxyURL(proxyURL)
		case "http", "https":
			dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Dial the proxy. For https:// proxies, we TLS-connect to the
				// proxy itself and force ALPN to HTTP/1.1 to prevent Go from
				// negotiating HTTP/2 for the CONNECT tunnel. Many proxy servers
				// don't support HTTP/2 CONNECT, and Go's default Transport.Proxy
				// would negotiate h2 via ALPN when TLS-connecting to an https://
				// proxy, causing "bogus greeting" errors. For http:// proxies,
				// CONNECT is always HTTP/1.1 over plain TCP so this isn't needed.
				// The target connection (e.g. to sourcegraph.com) still negotiates
				// HTTP/2 normally through the established tunnel.
				proxyAddr := proxyDialAddr(proxyURL)

				var conn net.Conn
				var err error
				if proxyURL.Scheme == "https" {
					raw, dialErr := (&net.Dialer{}).DialContext(ctx, "tcp", proxyAddr)
					if dialErr != nil {
						return nil, dialErr
					}
					cfg := baseTransport.TLSClientConfig.Clone()
					cfg.NextProtos = []string{"http/1.1"}
					if cfg.ServerName == "" {
						cfg.ServerName = proxyURL.Hostname()
					}
					tlsConn := tls.Client(raw, cfg)
					if err := tlsConn.HandshakeContext(ctx); err != nil {
						raw.Close()
						return nil, err
					}
					conn = tlsConn
				} else {
					conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", proxyAddr)
				}
				if err != nil {
					return nil, err
				}

				connectReq := &http.Request{
					Method: "CONNECT",
					URL:    &url.URL{Opaque: addr},
					Host:   addr,
					Header: make(http.Header),
				}
				if proxyURL.User != nil {
					password, _ := proxyURL.User.Password()
					auth := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + password))
					connectReq.Header.Set("Proxy-Authorization", "Basic "+auth)
				}
				if err := connectReq.Write(conn); err != nil {
					conn.Close()
					return nil, err
				}

				br := bufio.NewReader(conn)
				resp, err := http.ReadResponse(br, nil)
				if err != nil {
					conn.Close()
					return nil, err
				}
				if resp.StatusCode != http.StatusOK {
					// For non-200, it's safe/appropriate to close the body (it’s a real response body here).
					// Try to read a bit (4k bytes) to include in the error message.
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
					resp.Body.Close()
					conn.Close()
					return nil, fmt.Errorf("failed to connect to proxy %s: %s: %q", proxyURL.Redacted(), resp.Status, b)
				}
				// 200 CONNECT: do NOT resp.Body.Close(); it would interfere with the tunnel.
				return &connWithBufferedReader{Conn: conn, r: br}, nil
			}
			dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Dial the underlying connection through the proxy
				conn, err := dial(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return handshakeTLS(ctx, conn, addr)
			}
			baseTransport.DialContext = dial
			baseTransport.DialTLSContext = dialTLS
			// clear out any system proxy settings
			baseTransport.Proxy = nil
		}
	}

	return baseTransport
}
