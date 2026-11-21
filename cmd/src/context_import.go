package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const contextImportExamples = `
Create a search context from a software bill of materials (SBOM).

Each package in the SBOM must reference its Git repository and revision. In an
SPDX 3.0 SBOM, give the package an external reference of type "vcs" whose
locator is "<clone URL>@<revision>". In a CycloneDX SBOM, give the component an
external reference of type "vcs" whose URL is "<clone URL>@<revision>". Packages
without a repository and revision, or whose repository is not on the Sourcegraph
instance, are skipped and listed in the output.

The search context is owned by the current user. Search it with
"context:@<username>/<name>".

Examples:

  Create a private search context named spring-sbom from an SBOM file:

    	$ src context import -name=spring-sbom -f=sbom.cdx.json

  Create a public search context with a description from an SBOM read from stdin:

    	$ cat sbom.spdx.json | src context import -name=spring-sbom -description='Spring Boot sources' -public -f=-
`

var contextImportCommand = clicompat.Wrap(&cli.Command{
	Name:        "import",
	Usage:       "creates a search context from an SBOM",
	UsageText:   "src context import [options]",
	Description: contextImportExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:      "name",
			Usage:     "The name of the new search context",
			Required:  true,
			Validator: requiresNotEmpty("provide a search context name using -name"),
		},
		&cli.StringFlag{
			Name:      "file",
			Aliases:   []string{"f"},
			Usage:     "SPDX 3.0 or CycloneDX JSON SBOM file to import (- for stdin)",
			TakesFile: true,
			Required:  true,
			Validator: requiresNotEmpty("provide an SBOM file using -file"),
		},
		&cli.StringFlag{
			Name:  "description",
			Usage: "The description of the new search context",
		},
		&cli.BoolFlag{
			Name:  "public",
			Usage: "Make the search context visible to everyone instead of only you",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		sbom, err := readFile(cmd.String("file"))
		if err != nil {
			return err
		}

		visibility := "VISIBILITY_PRIVATE"
		if cmd.Bool("public") {
			visibility = "VISIBILITY_PUBLIC"
		}

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)
		return runContextImport(ctx, client, importSbomRequest{
			SearchContextID: cmd.String("name"),
			SBOM:            string(sbom),
			Description:     cmd.String("description"),
			Visibility:      visibility,
		}, cmd.Writer)
	},
})

// importSbomRequest is the JSON form of searchcontexts.v1.ImportSbomRequest.
type importSbomRequest struct {
	SearchContextID string `json:"searchContextId"`
	SBOM            string `json:"sbom"`
	Description     string `json:"description,omitempty"`
	Visibility      string `json:"visibility"`
}

// importSbomResponse is the JSON form of searchcontexts.v1.ImportSbomResponse.
type importSbomResponse struct {
	SearchContext struct {
		Spec         string `json:"spec"`
		Repositories []struct {
			Repository string   `json:"repository"`
			Revisions  []string `json:"revisions"`
		} `json:"repositories"`
	} `json:"searchContext"`
	SkippedPackages []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		VCSURL      string `json:"vcsUrl"`
		Reason      string `json:"reason"`
	} `json:"skippedPackages"`
}

var skippedPackageReasons = map[string]string{
	"REASON_MISSING_VCS_REFERENCE": "no Git repository reference",
	"REASON_INVALID_VCS_REFERENCE": "invalid Git repository reference",
	"REASON_MISSING_REVISION":      "no revision in the Git repository reference",
	"REASON_REPOSITORY_NOT_FOUND":  "repository not found on this Sourcegraph instance",
}

func runContextImport(ctx context.Context, client api.Client, input importSbomRequest, out io.Writer) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := client.NewHTTPRequest(ctx, http.MethodPost, "api/searchcontexts.v1.Service/ImportSbom", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Connect-Protocol-Version", "1")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return connectError(resp.Status, respBody)
	}

	var result importSbomResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return errors.Wrap(err, "decoding response")
	}

	fmt.Fprintf(out, "Search context %q created with %d repositories.\n", result.SearchContext.Spec, len(result.SearchContext.Repositories))
	if len(result.SkippedPackages) > 0 {
		fmt.Fprintf(out, "\nSkipped %d packages:\n", len(result.SkippedPackages))
		for _, p := range result.SkippedPackages {
			reason := skippedPackageReasons[p.Reason]
			if reason == "" {
				reason = p.Reason
			}
			fmt.Fprintf(out, "  %s (id %q): %s\n", p.DisplayName, p.ID, reason)
		}
	}
	return nil
}

// connectError turns a Connect protocol error response into an error that
// includes the per-package reasons the server attaches as a google.rpc.BadRequest
// detail when nothing in the SBOM could be imported.
func connectError(status string, body []byte) error {
	var connectErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"details"`
	}
	if err := json.Unmarshal(body, &connectErr); err != nil || connectErr.Code == "" {
		if len(body) > 1024 {
			body = body[:1024]
		}
		return errors.Newf("error: %s\n\n%s", status, body)
	}

	var msg strings.Builder
	msg.WriteString(connectErr.Message)
	for _, d := range connectErr.Details {
		if d.Type != "google.rpc.BadRequest" {
			continue
		}
		// Connect encodes details as base64 without padding but accepts both forms.
		value, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(d.Value, "="))
		if err != nil {
			continue
		}
		var badRequest errdetails.BadRequest
		if err := proto.Unmarshal(value, &badRequest); err != nil {
			continue
		}
		for _, v := range badRequest.GetFieldViolations() {
			msg.WriteString("\n  ")
			msg.WriteString(v.GetDescription())
		}
	}
	return errors.New(msg.String())
}
