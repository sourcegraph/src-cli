package srcproxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthorizationForRequest_DefaultMode(t *testing.T) {
	t.Parallel()

	s := &Serve{
		AccessToken: "test-token",
		Info:        log.New(ioDiscard{}, "", 0),
		Debug:       log.New(ioDiscard{}, "", 0),
	}

	authHeader, err := s.authorizationForRequest(&http.Request{})
	if err != nil {
		t.Fatalf("authorizationForRequest() error = %v", err)
	}
	if got, want := authHeader, "token test-token"; got != want {
		t.Fatalf("auth header = %q, want %q", got, want)
	}
}

func TestAuthorizationForRequest_MTLSSuccess(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var payload struct {
			Query string `json:"query"`
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		if !strings.Contains(payload.Query, "LookupUserByEmail") {
			t.Fatalf("unexpected query: %s", payload.Query)
		}
		if got, want := r.Header.Get("Authorization"), "token token-1"; got != want {
			t.Fatalf("auth header = %q, want %q", got, want)
		}

		return testResponse(`{"data":{"user":{"username":"alice"}}}`), nil
	})}

	s := &Serve{
		Endpoint:     "https://example.com",
		AccessToken:  "token-1",
		ClientCAPath: "ca.pem",
		httpClient:   client,
		userByEmail:  map[string]string{},
		Info:         log.New(ioDiscard{}, "", 0),
		Debug:        log.New(ioDiscard{}, "", 0),
	}

	req := &http.Request{TLS: tlsStateWithEmail("alice@example.com")}
	authHeader, err := s.authorizationForRequest(req)
	if err != nil {
		t.Fatalf("authorizationForRequest() error = %v", err)
	}
	if got, want := authHeader, `token-sudo token="token-1",user="alice"`; got != want {
		t.Fatalf("auth header = %q, want %q", got, want)
	}
}

func TestVerifySudoCapability(t *testing.T) {
	t.Parallel()

	var step int
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch step {
		case 0:
			if got, want := r.Header.Get("Authorization"), "token token-1"; got != want {
				t.Fatalf("step 0 auth = %q, want %q", got, want)
			}
			step++
			return testResponse(`{"data":{"currentUser":{"username":"site-admin"}}}`), nil
		case 1:
			if got, want := r.Header.Get("Authorization"), `token-sudo token="token-1",user="site-admin"`; got != want {
				t.Fatalf("step 1 auth = %q, want %q", got, want)
			}
			step++
			return testResponse(`{"data":{"currentUser":{"username":"site-admin"}}}`), nil
		default:
			t.Fatalf("unexpected extra call")
			return nil, nil
		}
	})}

	s := &Serve{
		Endpoint:    "https://example.com",
		AccessToken: "token-1",
		httpClient:  client,
		Info:        log.New(ioDiscard{}, "", 0),
		Debug:       log.New(ioDiscard{}, "", 0),
	}

	if err := s.verifySudoCapability(context.Background()); err != nil {
		t.Fatalf("verifySudoCapability() error = %v", err)
	}
	if got, want := step, 2; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}
}

func TestLoadOrGenerateServerCert_FromFiles(t *testing.T) {
	t.Parallel()

	certPath, keyPath := writeServerCertKeyPair(t)
	s := &Serve{
		ServerCertPath: certPath,
		ServerKeyPath:  keyPath,
	}

	cert, err := s.loadOrGenerateServerCert()
	if err != nil {
		t.Fatalf("loadOrGenerateServerCert() error = %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected loaded certificate chain")
	}
}

func TestEmailFromPeerCertificates(t *testing.T) {
	t.Parallel()

	tlsState := tlsStateWithEmail("alice@example.com")
	email, err := emailFromPeerCertificates(tlsState)
	if err != nil {
		t.Fatalf("emailFromPeerCertificates() error = %v", err)
	}
	if got, want := email, "alice@example.com"; got != want {
		t.Fatalf("email = %q, want %q", got, want)
	}
}

func TestNewReverseProxy_RewritesHostAndHeaders(t *testing.T) {
	t.Parallel()

	endpointURL, err := url.Parse("https://sourcegraph.test:3443")
	if err != nil {
		t.Fatalf("parse endpoint URL: %v", err)
	}

	s := &Serve{
		AdditionalHeaders: map[string]string{"X-Test": "1"},
		Info:              log.New(ioDiscard{}, "", 0),
		Debug:             log.New(ioDiscard{}, "", 0),
	}
	proxy := s.newReverseProxy(endpointURL)

	req, err := http.NewRequest("POST", "https://localhost:7777/.api/graphql", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "localhost:7777"

	proxy.Director(req)

	if got, want := req.URL.Host, "sourcegraph.test:3443"; got != want {
		t.Fatalf("URL host = %q, want %q", got, want)
	}
	if got, want := req.Host, "sourcegraph.test:3443"; got != want {
		t.Fatalf("host header = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("X-Test"), "1"; got != want {
		t.Fatalf("X-Test header = %q, want %q", got, want)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func tlsStateWithEmail(email string) *tls.ConnectionState {
	return &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{EmailAddresses: []string{email}}},
	}
}

func writeServerCertKeyPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	dir := t.TempDir()
	certPath = filepath.Join(dir, "server-cert.pem")
	keyPath = filepath.Join(dir, "server-key.pem")
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certPath, keyPath
}
