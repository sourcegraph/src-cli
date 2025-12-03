// Package oauthdevice implements the OAuth 2.0 Device Authorization Grant (RFC 8628)
// for authenticating with Sourcegraph instances.
package oauthdevice

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
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type Client interface {
	Discover(ctx context.Context, endpoint string) (*OIDCConfiguration, error)
	Start(ctx context.Context, endpoint string, scopes []string) (*DeviceAuthResponse, error)
	Poll(ctx context.Context, endpoint, deviceCode string, interval time.Duration, expiresIn int) (*TokenResponse, error)
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

	// Discover OIDC configuration
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

	// Discover OIDC configuration (should be cached from Start)
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
