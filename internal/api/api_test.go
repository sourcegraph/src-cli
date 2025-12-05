package api

import (
	"net/url"
	"testing"
)

func TestBuildTransport(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	t.Run("insecure skip verify", func(t *testing.T) {
		transport := buildTransport(ClientOpts{}, &Flags{insecureSkipVerify: boolPtr(true)})
		if !transport.TLSClientConfig.InsecureSkipVerify {
			t.Error("expected InsecureSkipVerify to be true")
		}
	})

	t.Run("unix socket proxy clears Proxy", func(t *testing.T) {
		transport := buildTransport(ClientOpts{ProxyPath: "/tmp/test.sock"}, defaultFlags())
		if transport.Proxy != nil {
			t.Error("expected Proxy to be nil")
		}
	})

	// http.DefaultTransport.Dial / DialTLS is already set and we can't compare two funcs
	// so our best effort here is to just check Proxy is nil / not nill based on the ProxyURL
	t.Run("http proxy clears Proxy", func(t *testing.T) {
		transport := buildTransport(ClientOpts{ProxyURL: &url.URL{Scheme: "http", Host: "proxy:8080"}}, defaultFlags())
		if transport.Proxy != nil {
			t.Error("expected Proxy to be nil")
		}
	})

	t.Run("socks5 proxy sets Proxy", func(t *testing.T) {
		transport := buildTransport(ClientOpts{ProxyURL: &url.URL{Scheme: "socks5", Host: "proxy:1080"}}, defaultFlags())
		if transport.Proxy == nil {
			t.Error("expected Proxy to be set")
		}
	})
}
