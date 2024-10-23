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

	"golang.org/x/net/proxy"
)

func applyProxy(transport *http.Transport, proxyEndpointURL *url.URL, proxyEndpointPath string) (applied bool) {
	if proxyEndpointURL == nil && proxyEndpointPath == "" {
		return false
	}

	handshakeTLS := func(conn net.Conn, addr string) (net.Conn, error) {
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
		if err := tlsConn.Handshake(); err != nil {
			return nil, err
		}
		return tlsConn, nil
	}

	var dial func(ctx context.Context, network, addr string) (net.Conn, error)
	var dialTLS func(ctx context.Context, network, addr string) (net.Conn, error)

	if proxyEndpointPath != "" {
		dial = func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", proxyEndpointPath)
		}
		dialTLS = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return handshakeTLS(conn, addr)
		}
	} else if proxyEndpointURL != nil {
		if proxyEndpointURL.Scheme == "socks5" ||
			proxyEndpointURL.Scheme == "socks5h" {
			dial = func(_ context.Context, network, addr string) (net.Conn, error) {
				// figure out the proxy every dial because we have error handling here.
				// In NewClient, we don't have error handling; all we can do there is panic.

				// FromURL really only handles SOCKS5 (unless other schemes have ben registered),
				// but since it also handles credentials,
				// we'll use it instead of manually handling any credentials embedded in the URL,
				// and calling SOCKS5 ourself.
				dialer, err := proxy.FromURL(proxyEndpointURL, proxy.Direct)
				if err != nil {
					return nil, err
				}
				return dialer.Dial(network, addr)
			}
			dialTLS = func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Dial the underlying connection through the proxy
				conn, err := dial(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return handshakeTLS(conn, addr)
			}
		} else if proxyEndpointURL.Scheme == "http" || proxyEndpointURL.Scheme == "https" {
			dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
				// separate the host from the port for the Host header
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				// Dial the proxy
				conn, err := net.Dial("tcp", proxyEndpointURL.Host)
				if err != nil {
					return nil, err
				}

				connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, host)

				// use authentication if proxy credentials are present
				if proxyEndpointURL.User != nil {
					password, _ := proxyEndpointURL.User.Password()
					auth := base64.StdEncoding.EncodeToString([]byte(proxyEndpointURL.User.Username() + ":" + password))
					connectReq += "Proxy-Authorization: Basic " + auth + "\r\n"
				}

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
					return nil, fmt.Errorf("failed to connect to proxy %v: %v", proxyEndpointURL, resp.Status)
				}
				resp.Body.Close()
				return conn, nil
			}
			dialTLS = func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Dial the underlying connection through the proxy
				conn, err := dial(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return handshakeTLS(conn, addr)
			}
		}
	}

	if dial != nil && dialTLS != nil {
		transport.DialContext = dial
		transport.DialTLSContext = dialTLS
	}

	return dial != nil && dialTLS != nil
}
