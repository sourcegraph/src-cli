package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/browser"

	"github.com/sourcegraph/sourcegraph/lib/accesstoken"
	"github.com/sourcegraph/sourcegraph/lib/codeintel/upload"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"
)

func init() {
	usage := `
Examples:
  Before running any of these, first use src auth to authenticate.
  Alternately, use the SRC_ACCESS_TOKEN environment variable for
  individual src-cli invocations.

  If run from within the project itself, src-cli will infer various
  flags based on git metadata.

        $ src code-intel upload # uploads ./index.scip

  If src-cli is invoked outside the project root, or if you're using
  a version control system other than git, specify flags explicitly:

    	$ src code-intel upload -root='' -repo=FOO -commit=BAR -file=index.scip

  Upload a SCIP index for a subproject:

    	$ src code-intel upload -root=cmd/

  Upload a SCIP index when lsif.enforceAuth is enabled in site settings:

    	$ src code-intel upload -github-token=BAZ, or
    	$ src code-intel upload -gitlab-token=BAZ
`
	codeintelCommands = append(codeintelCommands, &command{
		flagSet: codeintelUploadFlagSet,
		handler: handleCodeIntelUpload,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src code-intel %s':\n", codeintelUploadFlagSet.Name())
			codeintelUploadFlagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

// handleCodeIntelUpload is the handler for `src code-intel upload`.
func handleCodeIntelUpload(args []string) error {
	ctx := context.Background()

	out, err := parseAndValidateCodeIntelUploadFlags(args)
	if !codeintelUploadFlags.json {
		if out != nil {
			printInferredArguments(out)
		} else {
			// Always display inferred arguments except when -json is set
			printInferredArguments(emergencyOutput())
		}
	}
	if err != nil {
		return err
	}

	client := cfg.apiClient(codeintelUploadFlags.apiFlags, io.Discard)

	uploadOptions := codeintelUploadOptions(out)
	var uploadID int
	if codeintelUploadFlags.gzipCompressed {
		uploadID, err = UploadCompressedIndex(ctx, codeintelUploadFlags.file, client, uploadOptions, 0)
	} else {
		uploadID, err = UploadUncompressedIndex(ctx, codeintelUploadFlags.file, client, uploadOptions)
	}
	if err != nil {
		return handleUploadError(uploadOptions.SourcegraphInstanceOptions.AccessToken, err)
	}

	uploadURL, err := makeCodeIntelUploadURL(uploadID)
	if err != nil {
		return err
	}

	if codeintelUploadFlags.json {
		serialized, err := json.Marshal(map[string]any{
			"repo":           codeintelUploadFlags.repo,
			"commit":         codeintelUploadFlags.commit,
			"root":           codeintelUploadFlags.root,
			"file":           codeintelUploadFlags.file,
			"indexer":        codeintelUploadFlags.indexer,
			"indexerVersion": codeintelUploadFlags.indexerVersion,
			"uploadId":       uploadID,
			"uploadUrl":      uploadURL,
		})
		if err != nil {
			return err
		}

		fmt.Println(string(serialized))
	} else {
		if out == nil {
			out = emergencyOutput()
		}

		out.WriteLine(output.Linef(output.EmojiLightbulb, output.StyleItalic, "View processing status at %s", uploadURL))
	}

	if codeintelUploadFlags.open {
		if err := browser.OpenURL(uploadURL); err != nil {
			return err
		}
	}

	return nil
}

// codeintelUploadOptions creates a set of upload options given the values in the flags.
func codeintelUploadOptions(out *output.Output) upload.UploadOptions {
	var associatedIndexID *int
	if codeintelUploadFlags.associatedIndexID != -1 {
		associatedIndexID = &codeintelUploadFlags.associatedIndexID
	}

	cfg.AdditionalHeaders["Content-Type"] = "application/x-protobuf+scip"

	logger := upload.NewRequestLogger(
		os.Stdout,
		// Don't need to check upper bounds as we only compare verbosity ranges
		// It's fine if someone supplies -trace=42, but it will just behave the
		// same as if they supplied the highest verbosity level we define
		// internally.
		upload.RequestLoggerVerbosity(codeintelUploadFlags.verbosity),
	)

	return upload.UploadOptions{
		UploadRecordOptions: upload.UploadRecordOptions{
			Repo:              codeintelUploadFlags.repo,
			Commit:            codeintelUploadFlags.commit,
			Root:              codeintelUploadFlags.root,
			Indexer:           codeintelUploadFlags.indexer,
			IndexerVersion:    codeintelUploadFlags.indexerVersion,
			AssociatedIndexID: associatedIndexID,
		},
		SourcegraphInstanceOptions: upload.SourcegraphInstanceOptions{
			SourcegraphURL:      cfg.Endpoint,
			AccessToken:         cfg.AccessToken,
			AdditionalHeaders:   cfg.AdditionalHeaders,
			MaxRetries:          5,
			RetryInterval:       time.Second,
			Path:                codeintelUploadFlags.uploadRoute,
			MaxPayloadSizeBytes: codeintelUploadFlags.maxPayloadSizeMb * 1000 * 1000,
			MaxConcurrency:      codeintelUploadFlags.maxConcurrency,
			GitHubToken:         codeintelUploadFlags.gitHubToken,
			GitLabToken:         codeintelUploadFlags.gitLabToken,
		},
		OutputOptions: upload.OutputOptions{
			Output: out,
			Logger: logger,
		},
	}
}

// printInferredArguments prints a block showing the effective values of flags that are
// inferrably defined. This function is called on all paths except for -json uploads. This
// function no-ops if the given output object is nil.
func printInferredArguments(out *output.Output) {
	if out == nil {
		return
	}

	block := out.Block(output.Line(output.EmojiLightbulb, output.StyleItalic, "Inferred arguments"))
	block.Writef("repo: %s", codeintelUploadFlags.repo)
	block.Writef("commit: %s", codeintelUploadFlags.commit)
	block.Writef("root: %s", codeintelUploadFlags.root)
	block.Writef("file: %s", codeintelUploadFlags.file)
	block.Writef("indexer: %s", codeintelUploadFlags.indexer)
	block.Writef("indexerVersion: %s", codeintelUploadFlags.indexerVersion)
	block.Close()
}

// makeCodeIntelUploadURL constructs a URL to the upload with the given internal identifier.
// The base of the URL is constructed from the configured Sourcegraph instance.
func makeCodeIntelUploadURL(uploadID int) (string, error) {
	url, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return "", err
	}

	graphqlID := base64.URLEncoding.EncodeToString(fmt.Appendf(nil, `SCIPUpload:%d`, uploadID))
	url.Path = codeintelUploadFlags.repo + "/-/code-intelligence/uploads/" + graphqlID
	url.User = nil
	return url.String(), nil
}

type errorWithHint struct {
	err  error
	hint string
}

func (e errorWithHint) Error() string {
	return fmt.Sprintf("%s\n\n%s\n", e.err, e.hint)
}

// handleUploadError attaches actionable hints to upload errors and returns
// the enriched error. Only called for actual upload failures (not flag validation).
func handleUploadError(accessToken string, err error) error {
	if httpErr := findAuthError(err); httpErr != nil {
		isUnauthorized := httpErr.Code == 401
		isForbidden := httpErr.Code == 403

		displayErr := errors.Newf("upload failed: %s", uploadFailureReason(httpErr))

		err = errorWithHint{
			err:  displayErr,
			hint: uploadHints(accessToken, isUnauthorized, isForbidden),
		}
	}

	if codeintelUploadFlags.ignoreUploadFailures {
		fmt.Println(err.Error())
		return nil
	}

	return err
}

// findAuthError searches the error chain (including multi-errors from retries)
// for a 401 or 403 ErrUnexpectedStatusCode. Returns nil if none is found.
func findAuthError(err error) *ErrUnexpectedStatusCode {
	// Check if it's a multi-error and scan all children.
	if multi, ok := err.(errors.MultiError); ok {
		for _, e := range multi.Errors() {
			if found := findAuthError(e); found != nil {
				return found
			}
		}
		return nil
	}

	var httpErr *ErrUnexpectedStatusCode
	if errors.As(err, &httpErr) && (httpErr.Code == 401 || httpErr.Code == 403) {
		return httpErr
	}
	return nil
}

// uploadHints builds hint paragraphs for the Sourcegraph access token,
// code host tokens, and a docs link.
func uploadHints(accessToken string, isUnauthorized, isForbidden bool) string {
	var causes []string

	if h := sourcegraphAccessTokenHint(accessToken, isUnauthorized, isForbidden); h != "" {
		causes = append(causes, "- "+h)
	}

	for _, h := range codeHostTokenHints(isUnauthorized) {
		causes = append(causes, "- "+h)
	}

	var parts []string
	parts = append(parts, "Possible causes:\n"+strings.Join(causes, "\n"))
	parts = append(parts, "For more details on uploading SCIP indexes, see https://sourcegraph.com/docs/cli/references/code-intel/upload.")

	return strings.Join(parts, "\n\n")
}

// sourcegraphAccessTokenHint returns a hint about the Sourcegraph access token
// based on the error type and token state.
func sourcegraphAccessTokenHint(accessToken string, isUnauthorized, isForbidden bool) string {
	if isUnauthorized {
		if accessToken == "" {
			return "No Sourcegraph access token was provided. Set the SRC_ACCESS_TOKEN environment variable to a valid token."
		}
		if _, parseErr := accesstoken.ParsePersonalAccessToken(accessToken); parseErr != nil {
			return "The provided Sourcegraph access token does not match the expected format (sgp_<40 hex chars> or sgp_<instance-id>_<40 hex chars>). Was it copied incorrectly or truncated?"
		}
		return "The Sourcegraph access token may be invalid, expired, or you may be connecting to the wrong Sourcegraph instance."
	}
	if isForbidden {
		return "You may not have sufficient permissions on this Sourcegraph instance."
	}
	return ""
}

// codeHostTokenHints returns hints about GitHub or GitLab tokens.
func codeHostTokenHints(isUnauthorized bool) []string {
	if codeintelUploadFlags.gitHubToken != "" || strings.HasPrefix(codeintelUploadFlags.repo, "github.com") {
		return []string{gitHubTokenHint(isUnauthorized)}
	}
	if codeintelUploadFlags.gitLabToken != "" || strings.HasPrefix(codeintelUploadFlags.repo, "gitlab.com") {
		return []string{gitLabTokenHint(isUnauthorized)}
	}
	return []string{"Code host verification is supported for github.com and gitlab.com repositories."}
}

// gitHubTokenHint returns a hint about the GitHub token.
// Only called when gitHubToken is set or repo starts with "github.com".
func gitHubTokenHint(isUnauthorized bool) string {
	if codeintelUploadFlags.gitHubToken == "" {
		return fmt.Sprintf("No -github-token was provided. If this Sourcegraph instance enforces code host authentication, retry with -github-token=<token> for a token with access to %s.", codeintelUploadFlags.repo)
	}
	if isUnauthorized {
		return "The supplied -github-token may be invalid."
	}
	return "The supplied -github-token may lack the required permissions."
}

// gitLabTokenHint returns a hint about the GitLab token.
// Only called when gitLabToken is set or repo starts with "gitlab.com".
func gitLabTokenHint(isUnauthorized bool) string {
	if codeintelUploadFlags.gitLabToken == "" {
		return fmt.Sprintf("No -gitlab-token was provided. If this Sourcegraph instance enforces code host authentication, retry with -gitlab-token=<token> for a token with access to %s.", codeintelUploadFlags.repo)
	}
	if isUnauthorized {
		return "The supplied -gitlab-token may be invalid."
	}
	return "The supplied -gitlab-token may lack the required permissions."
}

// uploadFailureReason returns the server's response body if available, or a
// generic reason derived from the HTTP status code.
func uploadFailureReason(httpErr *ErrUnexpectedStatusCode) string {
	if httpErr.Body != "" {
		return httpErr.Body
	}
	if httpErr.Code == 401 {
		return "unauthorized"
	}
	return "forbidden"
}

// emergencyOutput creates a default Output object writing to standard out.
func emergencyOutput() *output.Output {
	return output.NewOutput(os.Stdout, output.OutputOpts{})
}
