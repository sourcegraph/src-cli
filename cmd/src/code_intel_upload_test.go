package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// validSGToken is a well-formed (but not real) Sourcegraph personal access token.
const validSGToken = "sgp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestUploadFailureReason(t *testing.T) {
	tests := []struct {
		name string
		err  *ErrUnexpectedStatusCode
		want string
	}{
		{"401 with body", &ErrUnexpectedStatusCode{Code: 401, Body: "Invalid access token."}, "Invalid access token."},
		{"401 without body", &ErrUnexpectedStatusCode{Code: 401}, "unauthorized"},
		{"403 with body", &ErrUnexpectedStatusCode{Code: 403, Body: "no write permission"}, "no write permission"},
		{"403 without body", &ErrUnexpectedStatusCode{Code: 403}, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, uploadFailureReason(tt.err))
		})
	}
}

func TestSourcegraphAccessTokenHint(t *testing.T) {
	tests := []struct {
		name           string
		accessToken    string
		isUnauthorized bool
		isForbidden    bool
		wantContains   string
	}{
		{
			name:           "401 no token",
			isUnauthorized: true,
			wantContains:   "No Sourcegraph access token was provided",
		},
		{
			name:           "401 malformed token",
			accessToken:    "not-a-valid-token",
			isUnauthorized: true,
			wantContains:   "does not match the expected format",
		},
		{
			name:           "401 valid format token",
			accessToken:    validSGToken,
			isUnauthorized: true,
			wantContains:   "may be invalid, expired",
		},
		{
			name:         "403",
			isForbidden:  true,
			wantContains: "may not have sufficient permissions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourcegraphAccessTokenHint(tt.accessToken, tt.isUnauthorized, tt.isForbidden)
			assert.Contains(t, got, tt.wantContains)
		})
	}
}

func TestCodeHostTokenHints(t *testing.T) {
	tests := []struct {
		name           string
		repo           string
		gitHubToken    string
		gitLabToken    string
		isUnauthorized bool
		wantContains   []string
	}{
		{
			name:           "github repo no token 401",
			repo:           "github.com/org/repo",
			isUnauthorized: true,
			wantContains:   []string{"No -github-token was provided", "github.com/org/repo"},
		},
		{
			name:           "github repo no token 403",
			repo:           "github.com/org/repo",
			isUnauthorized: false,
			wantContains:   []string{"No -github-token was provided"},
		},
		{
			name:           "github repo with token 401",
			repo:           "github.com/org/repo",
			gitHubToken:    "ghp_xxx",
			isUnauthorized: true,
			wantContains:   []string{"-github-token may be invalid"},
		},
		{
			name:           "github repo with token 403",
			repo:           "github.com/org/repo",
			gitHubToken:    "ghp_xxx",
			isUnauthorized: false,
			wantContains:   []string{"-github-token may lack the required permissions"},
		},
		{
			name:           "gitlab repo no token 401",
			repo:           "gitlab.com/org/repo",
			isUnauthorized: true,
			wantContains:   []string{"No -gitlab-token was provided", "gitlab.com/org/repo"},
		},
		{
			name:           "gitlab repo no token 403",
			repo:           "gitlab.com/org/repo",
			isUnauthorized: false,
			wantContains:   []string{"No -gitlab-token was provided"},
		},
		{
			name:           "gitlab repo with token 401",
			repo:           "gitlab.com/org/repo",
			gitLabToken:    "glpat-xxx",
			isUnauthorized: true,
			wantContains:   []string{"-gitlab-token may be invalid"},
		},
		{
			name:           "gitlab repo with token 403",
			repo:           "gitlab.com/org/repo",
			gitLabToken:    "glpat-xxx",
			isUnauthorized: false,
			wantContains:   []string{"-gitlab-token may lack the required permissions"},
		},
		{
			name:           "other repo no tokens",
			repo:           "bitbucket.org/org/repo",
			isUnauthorized: true,
			wantContains:   []string{"Code host verification is supported for github.com and gitlab.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := codeintelUploadFlags
			defer func() { codeintelUploadFlags = saved }()

			codeintelUploadFlags.repo = tt.repo
			codeintelUploadFlags.gitHubToken = tt.gitHubToken
			codeintelUploadFlags.gitLabToken = tt.gitLabToken

			hints := codeHostTokenHints(tt.isUnauthorized)
			joined := strings.Join(hints, "\n")
			for _, s := range tt.wantContains {
				assert.Contains(t, joined, s)
			}
		})
	}
}

func TestUploadHints(t *testing.T) {
	tests := []struct {
		name           string
		accessToken    string
		repo           string
		gitHubToken    string
		gitLabToken    string
		isUnauthorized bool
		isForbidden    bool
		wantContains   []string
	}{
		{
			name:           "401 no SG token github repo",
			repo:           "github.com/org/repo",
			isUnauthorized: true,
			wantContains: []string{
				"Possible causes:",
				"- No Sourcegraph access token was provided",
				"- No -github-token was provided",
				"sourcegraph.com/docs/cli/references/code-intel/upload",
			},
		},
		{
			name:           "401 valid SG token github token supplied",
			accessToken:    validSGToken,
			repo:           "github.com/org/repo",
			gitHubToken:    "ghp_xxx",
			isUnauthorized: true,
			wantContains: []string{
				"Possible causes:",
				"- The Sourcegraph access token may be invalid, expired",
				"- The supplied -github-token may be invalid",
				"sourcegraph.com/docs/cli/references/code-intel/upload",
			},
		},
		{
			name:        "403 gitlab token supplied",
			accessToken: validSGToken,
			repo:        "gitlab.com/org/repo",
			gitLabToken: "glpat-xxx",
			isForbidden: true,
			wantContains: []string{
				"Possible causes:",
				"- You may not have sufficient permissions",
				"- The supplied -gitlab-token may lack the required permissions",
				"sourcegraph.com/docs/cli/references/code-intel/upload",
			},
		},
		{
			name:           "401 bitbucket repo catch-all",
			accessToken:    validSGToken,
			repo:           "bitbucket.org/org/repo",
			isUnauthorized: true,
			wantContains: []string{
				"Possible causes:",
				"- The Sourcegraph access token may be invalid, expired",
				"- Code host verification is supported for github.com and gitlab.com",
				"sourcegraph.com/docs/cli/references/code-intel/upload",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := codeintelUploadFlags
			defer func() { codeintelUploadFlags = saved }()

			codeintelUploadFlags.repo = tt.repo
			codeintelUploadFlags.gitHubToken = tt.gitHubToken
			codeintelUploadFlags.gitLabToken = tt.gitLabToken

			got := uploadHints(tt.accessToken, tt.isUnauthorized, tt.isForbidden)
			for _, s := range tt.wantContains {
				assert.Contains(t, got, s)
			}
		})
	}
}

func TestHandleUploadError(t *testing.T) {
	tests := []struct {
		name         string
		accessToken  string
		repo         string
		gitHubToken  string
		err          error
		wantContains []string
		wantNil      bool
	}{
		{
			name:        "401 with server body",
			accessToken: validSGToken,
			repo:        "github.com/org/repo",
			err:         &ErrUnexpectedStatusCode{Code: 401, Body: "Invalid access token."},
			wantContains: []string{
				"upload failed: Invalid access token.",
				"Possible causes:",
				"- The Sourcegraph access token may be invalid, expired",
			},
		},
		{
			name:        "401 without body",
			accessToken: "",
			repo:        "github.com/org/repo",
			err:         &ErrUnexpectedStatusCode{Code: 401},
			wantContains: []string{
				"upload failed: unauthorized",
				"Possible causes:",
				"- No Sourcegraph access token was provided",
			},
		},
		{
			name:        "403 with server body",
			accessToken: validSGToken,
			repo:        "github.com/org/repo",
			gitHubToken: "ghp_xxx",
			err:         &ErrUnexpectedStatusCode{Code: 403, Body: "no write permission"},
			wantContains: []string{
				"upload failed: no write permission",
				"Possible causes:",
				"- You may not have sufficient permissions",
			},
		},
		{
			name:         "500 passthrough",
			accessToken:  validSGToken,
			repo:         "github.com/org/repo",
			err:          &ErrUnexpectedStatusCode{Code: 500, Body: "internal error"},
			wantContains: []string{"unexpected status code: 500"},
		},
		{
			name:         "non-http error passthrough",
			accessToken:  validSGToken,
			repo:         "github.com/org/repo",
			err:          errors.New("connection refused"),
			wantContains: []string{"connection refused"},
		},
		{
			name:        "combined 502 + 403 from retries",
			accessToken: validSGToken,
			repo:        "github.com/org/repo",
			gitHubToken: "ghp_xxx",
			err: errors.CombineErrors(
				&ErrUnexpectedStatusCode{Code: 502},
				&ErrUnexpectedStatusCode{Code: 403, Body: "no write permission"},
			),
			wantContains: []string{
				"upload failed: no write permission",
				"Possible causes:",
				"- You may not have sufficient permissions",
			},
		},
		{
			name:        "combined 502 + 401 from retries",
			accessToken: validSGToken,
			repo:        "github.com/org/repo",
			err: errors.CombineErrors(
				&ErrUnexpectedStatusCode{Code: 502},
				&ErrUnexpectedStatusCode{Code: 401, Body: "Invalid access token."},
			),
			wantContains: []string{
				"upload failed: Invalid access token.",
				"Possible causes:",
				"- The Sourcegraph access token may be invalid, expired",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := codeintelUploadFlags
			defer func() { codeintelUploadFlags = saved }()

			codeintelUploadFlags.repo = tt.repo
			codeintelUploadFlags.gitHubToken = tt.gitHubToken
			codeintelUploadFlags.ignoreUploadFailures = false

			got := handleUploadError(tt.accessToken, tt.err)
			require.NotNil(t, got)
			for _, s := range tt.wantContains {
				assert.Contains(t, got.Error(), s)
			}
		})
	}
}

func TestHandleUploadErrorIgnoreFailures(t *testing.T) {
	saved := codeintelUploadFlags
	defer func() { codeintelUploadFlags = saved }()

	codeintelUploadFlags.repo = "github.com/org/repo"
	codeintelUploadFlags.ignoreUploadFailures = true

	got := handleUploadError(validSGToken, &ErrUnexpectedStatusCode{Code: 401, Body: "bad token"})
	assert.Nil(t, got)
}
