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

func applyProxy(transport *http.Transport, ProxyURL *url.URL, ProxyPath string) (applied bool) {
	if ProxyURL == nil && ProxyPath == "" {
		return false
	}

	handshakeTLS := func(ctx context.Context, conn net.Conn, addr string) (net.Conn, error) {
		// Extract the hostname (without the port) for TLS SNI
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		// Be sure to clone TLS-specific config settings from the transport,
		// like InsecureSkipVerify.
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: transport.TLSClientConfig.InsecureSkipVerify,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		return tlsConn, nil
	}

	proxyApplied := false

	if ProxyPath != "" {
		dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", ProxyPath)
		}
		dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return handshakeTLS(ctx, conn, addr)
		}
		transport.DialContext = dial
		transport.DialTLSContext = dialTLS
		// clear out any system proxy settings
		transport.Proxy = nil
		proxyApplied = true
	} else if ProxyURL != nil {
		if ProxyURL.Scheme == "socks5" ||
			ProxyURL.Scheme == "socks5h" {
			// SOCKS proxies work out of the box - no need to manually dial
			transport.Proxy = http.ProxyURL(ProxyURL)
			proxyApplied = true
		} else if ProxyURL.Scheme == "http" || ProxyURL.Scheme == "https" {
			dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Dial the proxy
				d := net.Dialer{}
				conn, err := d.DialContext(ctx, "tcp", ProxyURL.Host)
				if err != nil {
					return nil, err
				}

				// this is the whole point of manually dialing the HTTP(S) proxy:
				// being able to force HTTP/1.
				// When relying on Transport.Proxy, the protocol is always HTTP/2,
				// but many proxy servers don't support HTTP/2.
				// We don't want to disable HTTP/2 in general because we want to use it when
				// connecting to the Sourcegraph API, using HTTP/1 for the proxy connection only.
				protocol := "HTTP/1.1"

				// CONNECT is the HTTP method used to set up a tunneling connection with a proxy
				method := "CONNECT"

				// Manually writing out the HTTP commands because it's not complicated,
				// and http.Request has some janky behavior:
				//   - ignores the Proto field and hard-codes the protocol to HTTP/1.1
				//   - ignores the Host Header (Header.Set("Host", host)) and uses URL.Host instead.
				//   - When the Host field is set, overrides the URL field
				connectReq := fmt.Sprintf("%s %s %s\r\n", method, addr, protocol)

				// A Host header is required per RFC 2616, section 14.23
				connectReq += fmt.Sprintf("Host: %s\r\n", addr)

				// use authentication if proxy credentials are present
				if ProxyURL.User != nil {
					password, _ := ProxyURL.User.Password()
					auth := base64.StdEncoding.EncodeToString([]byte(ProxyURL.User.Username() + ":" + password))
					connectReq += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth)
				}

				// finish up with an extra carriage return + newline, as per RFC 7230, section 3
				connectReq += "\r\n"

				// Send the CONNECT request to the proxy to establish the tunnel
				if _, err := conn.Write([]byte(connectReq)); err != nil {
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
					return nil, fmt.Errorf("failed to connect to proxy %v: %v", ProxyURL, resp.Status)
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
			transport.DialContext = dial
			transport.DialTLSContext = dialTLS
			// clear out any system proxy settings
			transport.Proxy = nil
			proxyApplied = true
		}
	}

	return proxyApplied
}
