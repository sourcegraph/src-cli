package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
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

// withProxyTransport modifies the given transport to handle proxying of unix, socks5 and http connections.
//
// Note: baseTransport is considered to be a clone created with transport.Clone()
//
// - If a the proxyPath is not empty, a unix socket proxy is created.
// - Otherwise, the proxyURL is used to determine if we should proxy socks5 / http connections
func withProxyTransport(baseTransport *http.Transport, proxyURL *url.URL, proxyPath string) *http.Transport {
	handshakeTLS := func(ctx context.Context, conn net.Conn, addr string) (net.Conn, error) {
		// Extract the hostname (without the port) for TLS SNI
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: host,
			// Pull InsecureSkipVerify from the target host transport
			// so that insecure-skip-verify flag settings are honored for the proxy server
			InsecureSkipVerify: baseTransport.TLSClientConfig.InsecureSkipVerify,
		})
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
				// Dial the proxy
				d := net.Dialer{}
				conn, err := d.DialContext(ctx, "tcp", proxyURL.Host)
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
