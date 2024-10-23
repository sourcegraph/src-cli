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

	proxyApplied := false

	if proxyEndpointPath != "" {
		dial := func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", proxyEndpointPath)
		}
		dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return handshakeTLS(conn, addr)
		}
		transport.DialContext = dial
		transport.DialTLSContext = dialTLS
		proxyApplied = true
	} else if proxyEndpointURL != nil {
		if proxyEndpointURL.Scheme == "socks5" ||
			proxyEndpointURL.Scheme == "socks5h" {
			// SOCKS proxies work out of the box - no need to manually dial
			proxyApplied = true
		} else if proxyEndpointURL.Scheme == "http" || proxyEndpointURL.Scheme == "https" {
			dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
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
			dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Dial the underlying connection through the proxy
				conn, err := dial(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return handshakeTLS(conn, addr)
			}
			transport.DialContext = dial
			transport.DialTLSContext = dialTLS
			proxyApplied = true
		}
	}

	return proxyApplied
}
