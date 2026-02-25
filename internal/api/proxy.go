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

				// Read and check the response from the proxy
				resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
				if err != nil {
					conn.Close()
					return nil, err
				}
				if resp.StatusCode != http.StatusOK {
					conn.Close()
					return nil, fmt.Errorf("failed to connect to proxy %v: %v", proxyURL, resp.Status)
				}
				resp.Body.Close()
				return conn, nil
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
