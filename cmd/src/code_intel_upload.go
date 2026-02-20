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
		return handleUploadError(err)
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
		return handleUploadError(err)
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

// handleUploadError writes the given error to the given output. If the
// given output object is nil then the error will be written to standard out.
//
// This method returns the error that should be passed back up to the runner.
func handleUploadError(err error) error {
	if errors.Is(err, ErrUnauthorized) {
		serverMessage := serverMessageFromError(err)
		if serverMessage != "" {
			err = errorWithHint{
				err:  errors.Newf("upload rejected by server: %s", serverMessage),
				hint: codeHostTokenHint(),
			}
		} else {
			err = errorWithHint{
				err:  errors.New("upload failed: unauthorized (check your Sourcegraph access token)"),
				hint: "To create a Sourcegraph access token, see https://sourcegraph.com/docs/cli/how-tos/creating_an_access_token.",
			}
		}
	} else if strings.Contains(err.Error(), "unexpected status code: 403") {
		serverMessage := serverMessageFrom403Error(err)
		displayMsg := "upload failed: forbidden"
		if serverMessage != "" {
			displayMsg = fmt.Sprintf("upload rejected by server: %s", serverMessage)
		}
		err = errorWithHint{
			err:  errors.New(displayMsg),
			hint: codeHostTokenHint(),
		}
	}

	if codeintelUploadFlags.ignoreUploadFailures {
		// Report but don't return the error
		fmt.Println(err.Error())
		return nil
	}

	return err
}

// serverMessageFromError extracts the detail string from a wrapped ErrUnauthorized.
// Returns "" if the error is the bare sentinel (no server message).
func serverMessageFromError(err error) string {
	msg := err.Error()
	sentinel := ErrUnauthorized.Error()
	if msg == sentinel {
		return ""
	}
	// errors.Wrap produces "detail: sentinel", so strip the suffix.
	if strings.HasSuffix(msg, ": "+sentinel) {
		return strings.TrimSuffix(msg, ": "+sentinel)
	}
	return ""
}

// serverMessageFrom403Error extracts the server body from a 403 error.
// The vendored code formats these as "unexpected status code: 403 (body)".
func serverMessageFrom403Error(err error) string {
	msg := err.Error()
	prefix := "unexpected status code: 403 ("
	if strings.HasPrefix(msg, prefix) && strings.HasSuffix(msg, ")") {
		return msg[len(prefix) : len(msg)-1]
	}
	return ""
}

// codeHostTokenHint returns documentation links relevant to the upload failure.
// Always includes the general upload docs, and adds code-host-specific token
// documentation if the user supplied a code host token flag.
func codeHostTokenHint() string {
	hint := "For more details on uploading SCIP indexes, see https://sourcegraph.com/docs/cli/references/code-intel/upload."
	if codeintelUploadFlags.gitHubToken != "" {
		hint += "\nIf the issue is related to your GitHub token, see https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens."
	}
	if codeintelUploadFlags.gitLabToken != "" {
		hint += "\nIf the issue is related to your GitLab token, see https://docs.gitlab.com/user/profile/personal_access_tokens/."
	}
	return hint
}

// emergencyOutput creates a default Output object writing to standard out.
func emergencyOutput() *output.Output {
	return output.NewOutput(os.Stdout, output.OutputOpts{})
}


