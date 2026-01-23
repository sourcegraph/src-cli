// Package oauthimplements the OAuth 2.0 Device Authorization Grant (RFC 8628)
// for authenticating with Sourcegraph instances.
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/secrets"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const (
	// DefaultClientID is a predefined Client ID built into Sourcegraph
	DefaultClientID = "sgo_cid_sourcegraph-cli"

	// wellKnownPath is the path on the sourcegraph server where clients can discover OAuth configuration
	wellKnownPath = "/.well-known/openid-configuration"

	GrantTypeDeviceCode string = "urn:ietf:params:oauth:grant-type:device_code"

	ScopeOpenID        string = "openid"
	ScopeProfile       string = "profile"
	ScopeEmail         string = "email"
	ScopeOfflineAccess string = "offline_access"
	ScopeUserAll       string = "user:all"

	// storeKeyFmt is the format of the key name that will be used to store a value
	// typically the last element is the endpoint the value is for ie. src:oauth:https://sourcegraph.sourcegraph.com
	storeKeyFmt string = "src:oauth:%s"
)

var defaultScopes = []string{ScopeEmail, ScopeOfflineAccess, ScopeOpenID, ScopeProfile, ScopeUserAll}

// OIDCConfiguration represents the relevant fields from the OpenID Connect
// Discovery document at /.well-known/openid-configuration
type OIDCConfiguration struct {
	Issuer                      string `json:"issuer,omitempty"`
	TokenEndpoint               string `json:"token_endpoint,omitempty"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint,omitempty"`
}

type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
}

type Token struct {
	Endpoint     string    `json:"endpoint"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type Client interface {
	Discover(ctx context.Context, endpoint string) (*OIDCConfiguration, error)
	Start(ctx context.Context, endpoint string, scopes []string) (*DeviceAuthResponse, error)
	Poll(ctx context.Context, endpoint, deviceCode string, interval time.Duration, expiresIn int) (*TokenResponse, error)
	Refresh(ctx context.Context, token *Token) (*TokenResponse, error)
}

type httpClient struct {
	clientID string
	client   *http.Client
	// cached OIDC configuration per endpoint
	configCache map[string]*OIDCConfiguration
}

func NewClient(clientID string) Client {
	return &httpClient{
		clientID: clientID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		configCache: make(map[string]*OIDCConfiguration),
	}
}

func NewClientWithHTTPClient(c *http.Client) Client {
	return &httpClient{
		client:      c,
		configCache: make(map[string]*OIDCConfiguration),
	}
}

// Discover fetches the openid-configuration which contains all the routes a client should
// use for authorization, device flows, tokens etc.
//
// Before making any requests, the configCache is checked and if there is a cache hit, the
// cached config is returned.
func (c *httpClient) Discover(ctx context.Context, endpoint string) (*OIDCConfiguration, error) {
	endpoint = strings.TrimRight(endpoint, "/")

	if config, ok := c.configCache[endpoint]; ok {
		return config, nil
	}

	reqURL := endpoint + wellKnownPath

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating discovery request")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "discovery request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading discovery response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf("discovery failed with status %d: %s", resp.StatusCode, string(body))
	}

	var config OIDCConfiguration
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, errors.Wrap(err, "parsing discovery response")
	}

	c.configCache[endpoint] = &config

	return &config, nil
}

// Start starts the OAuth device flow with the given endpoint. If no scopes are given the default scopes are used.
//
// Default Scopes: "openid" "profile" "email" "offline_access" "user:all"
func (c *httpClient) Start(ctx context.Context, endpoint string, scopes []string) (*DeviceAuthResponse, error) {
	endpoint = strings.TrimRight(endpoint, "/")

	// Discover OIDC configuration - caches on first call
	config, err := c.Discover(ctx, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "OIDC discovery failed")
	}

	if config.DeviceAuthorizationEndpoint == "" {
		return nil, errors.New("device authorization endpoint not found in OIDC configuration; the server may not support device flow")
	}

	data := url.Values{}
	data.Set("client_id", DefaultClientID)
	if len(scopes) > 0 {
		data.Set("scope", strings.Join(scopes, " "))
	} else {
		data.Set("scope", strings.Join(defaultScopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.DeviceAuthorizationEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "creating device auth request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "device auth request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading device auth response")
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return nil, errors.Newf("device auth failed: %s: %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, errors.Newf("device auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp DeviceAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, errors.Wrap(err, "parsing device auth response")
	}

	return &authResp, nil
}

// Poll polls the OAuth token endpoint until the device has been authorized or not
//
// We poll as long as the authorization is pending. If the server tells us to slow down, we will wait 5 secs extra.
//
// Polling will stop when:
// - Device is authorized, and a token is returned
// - Device code has expried
// - User denied authorization
func (c *httpClient) Poll(ctx context.Context, endpoint, deviceCode string, interval time.Duration, expiresIn int) (*TokenResponse, error) {
	endpoint = strings.TrimRight(endpoint, "/")

	// Discover OIDC configuration - caches on first call
	config, err := c.Discover(ctx, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "OIDC discovery failed")
	}

	if config.TokenEndpoint == "" {
		return nil, errors.New("token endpoint not found in OIDC configuration")
	}

	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return nil, errors.New("device code expired")
		}

		if !testing.Testing() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}

		tokenResp, err := c.pollOnce(ctx, config.TokenEndpoint, deviceCode)
		if err != nil {
			var pollErr *PollError
			if errors.As(err, &pollErr) {
				switch pollErr.Code {
				case "authorization_pending":
					continue
				case "slow_down":
					interval += 5 * time.Second
					continue
				case "expired_token":
					return nil, errors.New("device code expired")
				case "access_denied":
					return nil, errors.New("authorization was denied by the user")
				}
			}
			return nil, err
		}

		return tokenResp, nil
	}
}

type PollError struct {
	Code        string
	Description string
}

func (e *PollError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Description)
	}
	return e.Code
}

func (c *httpClient) pollOnce(ctx context.Context, tokenEndpoint, deviceCode string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", DefaultClientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", GrantTypeDeviceCode)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "creating token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "token request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading token response")
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return nil, &PollError{Code: errResp.Error, Description: errResp.ErrorDescription}
		}
		return nil, errors.Newf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, errors.Wrap(err, "parsing token response")
	}

	return &tokenResp, nil
}

// Refresh exchanges a refresh token for a new access token.
func (c *httpClient) Refresh(ctx context.Context, token *Token) (*TokenResponse, error) {
	config, err := c.Discover(ctx, token.Endpoint)
	if err != nil {
		errors.Wrap(err, "failed to discover OIDC configuration")
	}

	if config.TokenEndpoint == "" {
		errors.New("OIDC configuration has no token endpoint")
	}

	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", token.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "creating refresh token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "refresh token request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading refresh token response")
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return nil, errors.Newf("refresh token failed: %s: %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, errors.Newf("refresh token failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, errors.Wrap(err, "parsing refresh token response")
	}

	return &tokenResp, err
}

func (t *TokenResponse) Token(endpoint string) *Token {
	return &Token{
		Endpoint:     strings.TrimRight(endpoint, "/"),
		RefreshToken: t.RefreshToken,
		AccessToken:  t.AccessToken,
		ExpiresAt:    time.Now().Add(time.Second * time.Duration(t.ExpiresIn)),
	}
}

func (t *Token) HasExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

func (t *Token) ExpiringIn(d time.Duration) bool {
	future := time.Now().Add(d)
	return future.After(t.ExpiresAt)
}

func oauthKey(endpoint string) string {
	return fmt.Sprintf(storeKeyFmt, endpoint)
}

func StoreToken(ctx context.Context, token *Token) error {
	store, err := secrets.Open(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(token)
	if err != nil {
		return errors.Wrap(err, "failed to marshal token")
	}

	if token.Endpoint == "" {
		return errors.New("token endpoint cannot be empty when storing the token")
	}

	return store.Put(oauthKey(token.Endpoint), data)
}

func LoadToken(ctx context.Context, endpoint string) (*Token, error) {
	store, err := secrets.Open(ctx)
	if err != nil {
		return nil, err
	}

	key := oauthKey(endpoint)
	data, err := store.Get(key)
	if err != nil {
		return nil, err
	}

	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshall token")
	}

	return &t, nil
}
