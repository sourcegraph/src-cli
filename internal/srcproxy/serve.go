package srcproxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type Serve struct {
	Addr               string
	Endpoint           string
	AccessToken        string
	ClientCAPath       string
	ServerCertPath     string
	ServerKeyPath      string
	InsecureSkipVerify bool
	AdditionalHeaders  map[string]string
	HTTPClient         *http.Client
	Info               *log.Logger
	Debug              *log.Logger

	mu           sync.Mutex
	userByEmail  map[string]string
	httpClient   *http.Client
	baseAuthMode string
}

func (s *Serve) Start() error {
	if s.AccessToken == "" {
		return errors.New("SRC_ACCESS_TOKEN must be set")
	}
	if s.Endpoint == "" {
		return errors.New("SRC_ENDPOINT must be set")
	}

	s.httpClient = s.HTTPClient
	if s.httpClient == nil {
		s.httpClient = &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
		if s.InsecureSkipVerify {
			s.httpClient.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
	}

	s.userByEmail = map[string]string{}

	if s.ClientCAPath == "" {
		s.baseAuthMode = "access-token"
	} else {
		if err := s.verifySudoCapability(context.Background()); err != nil {
			return err
		}
		s.baseAuthMode = "mtls-sudo"
	}

	endpointURL, err := url.Parse(strings.TrimRight(s.Endpoint, "/"))
	if err != nil {
		return errors.Wrap(err, "parse endpoint")
	}

	proxy := s.newReverseProxy(endpointURL)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Debug.Printf("incoming request method=%s host=%s path=%s remote=%s", r.Method, r.Host, r.URL.RequestURI(), r.RemoteAddr)
		authHeader, err := s.authorizationForRequest(r)
		if err != nil {
			s.Debug.Printf("authorization failed method=%s path=%s err=%s", r.Method, r.URL.Path, err)
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		r.Header.Set("Authorization", authHeader)
		s.Debug.Printf("proxying request method=%s path=%s upstream_host=%s", r.Method, r.URL.Path, endpointURL.Host)
		proxy.ServeHTTP(w, r)
	})

	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return errors.Wrap(err, "listen")
	}

	if s.ClientCAPath != "" {
		tlsCfg, err := s.mtlsServerTLSConfig()
		if err != nil {
			return err
		}
		ln = tls.NewListener(ln, tlsCfg)
	}

	s.Addr = ln.Addr().String()
	if s.ClientCAPath == "" {
		s.Info.Printf("listening on http://%s", s.Addr)
	} else {
		s.Info.Printf("listening on https://%s", s.Addr)
		s.Info.Printf("mTLS client CA: %s", s.ClientCAPath)
	}
	s.Info.Printf("proxying requests to %s", s.Endpoint)
	s.Info.Printf("auth mode: %s", s.baseAuthMode)

	if err := (&http.Server{Handler: handler}).Serve(ln); err != nil {
		return errors.Wrap(err, "serve")
	}

	return nil
}

func (s *Serve) newReverseProxy(endpointURL *url.URL) *httputil.ReverseProxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if s.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	proxy := httputil.NewSingleHostReverseProxy(endpointURL)
	proxy.Transport = transport

	upstreamDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		upstreamDirector(r)
		// Rewrite Host for name-based routing upstream (e.g. Caddy vhosts).
		r.Host = endpointURL.Host
		for key, value := range s.AdditionalHeaders {
			r.Header.Set(key, value)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.Debug.Printf("proxy request failed method=%s path=%s err=%s", r.Method, r.URL.Path, err)
		http.Error(w, "proxy request failed: "+err.Error(), http.StatusBadGateway)
	}
	return proxy
}

func (s *Serve) authorizationForRequest(r *http.Request) (string, error) {
	if s.ClientCAPath == "" {
		return "token " + s.AccessToken, nil
	}

	email, err := emailFromPeerCertificates(r.TLS)
	if err != nil {
		return "", err
	}

	username, err := s.lookupUsernameByEmail(r.Context(), email)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`token-sudo token=%q,user=%q`, s.AccessToken, username), nil
}

func (s *Serve) verifySudoCapability(ctx context.Context) error {
	const queryCurrentUser = `query CurrentUser { currentUser { username } }`

	var current struct {
		CurrentUser *struct {
			Username string `json:"username"`
		} `json:"currentUser"`
	}
	if err := doGraphQL(ctx, s.httpClient, s.Endpoint, "token "+s.AccessToken, queryCurrentUser, nil, &current); err != nil {
		return errors.Wrap(err, "verify base access token")
	}
	if current.CurrentUser == nil || current.CurrentUser.Username == "" {
		return errors.New("unable to resolve current user from access token")
	}

	sudoAuth := fmt.Sprintf(`token-sudo token=%q,user=%q`, s.AccessToken, current.CurrentUser.Username)
	if err := doGraphQL(ctx, s.httpClient, s.Endpoint, sudoAuth, queryCurrentUser, nil, &current); err != nil {
		return errors.Wrap(err, "verify token has site-admin:sudo scope")
	}

	return nil
}

func (s *Serve) lookupUsernameByEmail(ctx context.Context, email string) (string, error) {
	s.mu.Lock()
	if username, ok := s.userByEmail[email]; ok {
		s.mu.Unlock()
		return username, nil
	}
	s.mu.Unlock()

	username, err := lookupUserByEmail(ctx, s.httpClient, s.Endpoint, s.AccessToken, email)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.userByEmail[email] = username
	s.mu.Unlock()
	return username, nil
}

func (s *Serve) mtlsServerTLSConfig() (*tls.Config, error) {
	caData, err := os.ReadFile(s.ClientCAPath)
	if err != nil {
		return nil, errors.Wrap(err, "read mTLS client CA certificate")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, errors.New("failed to parse mTLS client CA certificate")
	}

	serverCert, err := s.loadOrGenerateServerCert()
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func (s *Serve) loadOrGenerateServerCert() (tls.Certificate, error) {
	if s.ServerCertPath != "" || s.ServerKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(s.ServerCertPath, s.ServerKeyPath)
		if err != nil {
			return tls.Certificate{}, errors.Wrap(err, "load server TLS certificate/key")
		}
		return cert, nil
	}

	cert, err := generateEphemeralServerCert()
	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "generate server TLS certificate")
	}
	return cert, nil
}

func generateEphemeralServerCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := time.Now().Add(24 * time.Hour)

	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "src-proxy",
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func emailFromPeerCertificates(tlsState *tls.ConnectionState) (string, error) {
	if tlsState == nil || len(tlsState.PeerCertificates) == 0 {
		return "", errors.New("no client certificate presented")
	}
	cert := tlsState.PeerCertificates[0]
	if len(cert.EmailAddresses) == 0 {
		return "", errors.New("client certificate does not contain an email SAN")
	}
	return cert.EmailAddresses[0], nil
}

func lookupUserByEmail(ctx context.Context, client *http.Client, endpoint, accessToken, email string) (string, error) {
	const query = `query LookupUserByEmail($email: String) {
  user(email: $email) {
    username
  }
}`

	var result struct {
		User *struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := doGraphQL(ctx, client, endpoint, "token "+accessToken, query, map[string]any{"email": email}, &result); err != nil {
		return "", err
	}
	if result.User == nil || result.User.Username == "" {
		return "", errors.New("no Sourcegraph user found for certificate email")
	}
	return result.User.Username, nil
}

func doGraphQL(ctx context.Context, client *http.Client, endpoint, authorizationHeader, query string, variables map[string]any, result any) error {
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return errors.Wrap(err, "marshal GraphQL request")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(endpoint, "/")+"/.api/graphql", bytes.NewReader(payload))
	if err != nil {
		return errors.Wrap(err, "create GraphQL request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authorizationHeader)

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "perform GraphQL request")
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read GraphQL response")
	}

	if resp.StatusCode != http.StatusOK {
		return errors.Newf("GraphQL request failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errors.Wrap(err, "decode GraphQL response envelope")
	}

	if len(envelope.Errors) > 0 {
		var messages []string
		for _, graphqlErr := range envelope.Errors {
			messages = append(messages, graphqlErr.Message)
		}
		return errors.New(strings.Join(messages, "; "))
	}

	if result == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, result); err != nil {
		return errors.Wrap(err, "decode GraphQL response data")
	}
	return nil
}
