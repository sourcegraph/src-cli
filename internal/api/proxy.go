package api

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
)

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
		tlsConn := tls.Client(conn, cfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tlsConn.Close()
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
		baseTransport.Proxy = http.ProxyURL(proxyURL)
	}

	return baseTransport
}
