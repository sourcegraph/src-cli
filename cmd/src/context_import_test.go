package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	"github.com/sourcegraph/src-cli/internal/api"
)

func TestRunContextImport(t *testing.T) {
	newClient := func(t *testing.T, handler http.HandlerFunc) api.Client {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		endpoint, err := url.Parse(server.URL)
		require.NoError(t, err)
		return api.NewClient(api.ClientOpts{
			EndpointURL: endpoint,
			AccessToken: "abc",
			Flags:       api.NewFlagsFromValues(false, false, false, false, false),
			Out:         io.Discard,
		})
	}

	t.Run("creates search context and lists skipped packages", func(t *testing.T) {
		client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/searchcontexts.v1.Service/ImportSbom", r.URL.Path)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			require.Equal(t, "token abc", r.Header.Get("Authorization"))

			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, map[string]any{
				"searchContextId": "spring-sbom",
				"sbom":            `{"bomFormat":"CycloneDX"}`,
				"visibility":      "VISIBILITY_PUBLIC",
			}, body)

			_, _ = io.WriteString(w, `{
				"searchContext": {
					"name": "users/1/searchContexts/spring-sbom",
					"spec": "@alice/spring-sbom",
					"visibility": "VISIBILITY_PUBLIC",
					"repositories": [
						{"repository": "github.com/spring-projects/spring-boot", "revisions": ["refs/tags/v4.1.1"]},
						{"repository": "github.com/spring-projects/spring-framework", "revisions": ["v7.0.0"]}
					]
				},
				"skippedPackages": [
					{"id": "left-pad", "displayName": "left-pad@1.3.0", "reason": "REASON_MISSING_VCS_REFERENCE"}
				]
			}`)
		})

		var out bytes.Buffer
		err := runContextImport(context.Background(), client, importSbomRequest{
			SearchContextID: "spring-sbom",
			SBOM:            `{"bomFormat":"CycloneDX"}`,
			Visibility:      "VISIBILITY_PUBLIC",
		}, &out)
		require.NoError(t, err)
		require.Equal(t, `Search context "@alice/spring-sbom" created with 2 repositories.

Skipped 1 packages:
  left-pad@1.3.0 (id "left-pad"): no Git repository reference
`, out.String())
	})

	t.Run("reports why nothing could be imported", func(t *testing.T) {
		detail, err := proto.Marshal(&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
			{Field: "sbom", Description: `left-pad@1.3.0 (id "left-pad"): REASON_MISSING_VCS_REFERENCE`},
			{Field: "sbom", Description: `internal-lib@2.0 (id "internal-lib"): REASON_REPOSITORY_NOT_FOUND`},
		}})
		require.NoError(t, err)

		client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    "failed_precondition",
				"message": "none of the 2 packages in the SBOM has a Git repository and revision on this Sourcegraph instance",
				"details": []map[string]string{{
					"type":  "google.rpc.BadRequest",
					"value": base64.RawStdEncoding.EncodeToString(detail),
				}},
			})
		})

		var out bytes.Buffer
		err = runContextImport(context.Background(), client, importSbomRequest{SearchContextID: "x", SBOM: "{}"}, &out)
		require.EqualError(t, err, `none of the 2 packages in the SBOM has a Git repository and revision on this Sourcegraph instance
  left-pad@1.3.0 (id "left-pad"): REASON_MISSING_VCS_REFERENCE
  internal-lib@2.0 (id "internal-lib"): REASON_REPOSITORY_NOT_FOUND`)
		require.Empty(t, out.String())
	})
}
