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
	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  Upload a SCIP index with explicit repo, commit, and upload files:

    	$ src upload -repo=FOO -commit=BAR -file=index.scip

  Upload a SCIP index for a subproject:

    	$ src upload -root=cmd/

  Upload a SCIP index when lsifEnforceAuth is enabled:

    	$ src upload -github-token=BAZ, or
    	$ src upload -gitlab-token=BAZ

  Upload an LSIF index when the LSIF indexer does not not declare a tool name.

    	$ src upload -indexer=lsif-elixir

  For any of these commands, an LSIF index (default name: dump.lsif) can be
  used instead of a SCIP index (default name: index.scip).
`
	commands = append(commands, &command{
		flagSet: uploadFlagSet,
		handler: handleUpload,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src %s':\n", uploadFlagSet.Name())
			uploadFlagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})

	// Make 'upload' available under 'src lsif' for backwards compatibility.
	lsifCommands = append(lsifCommands, &command{
		flagSet: uploadFlagSet,
		handler: handleUpload,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src lsif %s':\n", uploadFlagSet.Name())
			uploadFlagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

// handleUpload is the handler for `src upload`.
func handleUpload(args []string) error {
	ctx := context.Background()

	out, err := parseAndValidateUploadFlags(args)
	if !uploadFlags.json {
		if out != nil {
			printInferredArguments(out)
		} else {
			// Always display inferred arguments except when -json is set
			printInferredArguments(emergencyOutput())
		}
	}
	if err != nil {
		return handleUploadError(nil, err)
	}

	client := api.NewClient(api.ClientOpts{
		Out:   io.Discard,
		Flags: uploadFlags.apiFlags,
	})

	uploadID, err := upload.UploadIndex(ctx, uploadFlags.file, client, uploadOptions(out))
	if err != nil {
		return handleUploadError(out, err)
	}

	uploadURL, err := makeUploadURL(uploadID)
	if err != nil {
		return err
	}

	if uploadFlags.json {
		serialized, err := json.Marshal(map[string]interface{}{
			"repo":           uploadFlags.repo,
			"commit":         uploadFlags.commit,
			"root":           uploadFlags.root,
			"file":           uploadFlags.file,
			"indexer":        uploadFlags.indexer,
			"indexerVersion": uploadFlags.indexerVersion,
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

	if uploadFlags.open {
		if err := browser.OpenURL(uploadURL); err != nil {
			return err
		}
	}

	return nil
}

// uploadOptions creates a set of upload options given the values in the flags.
func uploadOptions(out *output.Output) upload.UploadOptions {
	var associatedIndexID *int
	if uploadFlags.associatedIndexID != -1 {
		associatedIndexID = &uploadFlags.associatedIndexID
	}

	logger := upload.NewRequestLogger(
		os.Stdout,
		// Don't need to check upper bounds as we only compare verbosity ranges
		// It's fine if someone supplies -trace=42, but it will just behave the
		// same as if they supplied the highest verbosity level we define
		// internally.
		upload.RequestLoggerVerbosity(uploadFlags.verbosity),
	)

	return upload.UploadOptions{
		UploadRecordOptions: upload.UploadRecordOptions{
			Repo:              uploadFlags.repo,
			Commit:            uploadFlags.commit,
			Root:              uploadFlags.root,
			Indexer:           uploadFlags.indexer,
			IndexerVersion:    uploadFlags.indexerVersion,
			AssociatedIndexID: associatedIndexID,
		},
		SourcegraphInstanceOptions: upload.SourcegraphInstanceOptions{
			SourcegraphURL:      cfg.Endpoint,
			AccessToken:         cfg.AccessToken,
			AdditionalHeaders:   cfg.AdditionalHeaders,
			MaxRetries:          5,
			RetryInterval:       time.Second,
			Path:                uploadFlags.uploadRoute,
			MaxPayloadSizeBytes: uploadFlags.maxPayloadSizeMb * 1000 * 1000,
			GitHubToken:         uploadFlags.gitHubToken,
			GitLabToken:         uploadFlags.gitLabToken,
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
	block.Writef("repo: %s", uploadFlags.repo)
	block.Writef("commit: %s", uploadFlags.commit)
	block.Writef("root: %s", uploadFlags.root)
	block.Writef("file: %s", uploadFlags.file)
	block.Writef("indexer: %s", uploadFlags.indexer)
	block.Writef("indexerVersion: %s", uploadFlags.indexerVersion)
	block.Close()
}

// makeUploadURL constructs a URL to the upload with the given internal identifier.
// The base of the URL is constructed from the configured Sourcegraph instance.
func makeUploadURL(uploadID int) (string, error) {
	url, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return "", err
	}

	graphqlID := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf(`LSIFUpload:"%d"`, uploadID)))
	url.Path = uploadFlags.repo + "/-/code-intelligence/uploads/" + graphqlID
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
func handleUploadError(out *output.Output, err error) error {
	if err == upload.ErrUnauthorized {
		err = filterLSIFUnauthorizedError(out, err)
	}

	if uploadFlags.ignoreUploadFailures {
		// Report but don't return the error
		fmt.Println(err.Error())
		return nil
	}

	return err
}

func filterLSIFUnauthorizedError(out *output.Output, err error) error {
	var actionableHints []string
	needsGitHubToken := strings.HasPrefix(uploadFlags.repo, "github.com")
	needsGitLabToken := strings.HasPrefix(uploadFlags.repo, "gitlab.com")

	if needsGitHubToken {
		if uploadFlags.gitHubToken != "" {
			actionableHints = append(actionableHints,
				fmt.Sprintf("The supplied -github-token does not indicate that you have collaborator access to %s.", uploadFlags.repo),
				"Please check the value of the supplied token and its permissions on the code host and try again.",
			)
		} else {
			actionableHints = append(actionableHints,
				fmt.Sprintf("Please retry your request with a -github-token=XXX with with collaborator access to %s.", uploadFlags.repo),
				"This token will be used to check with the code host that the uploading user has write access to the target repository.",
			)
		}
	} else if needsGitLabToken {
		if uploadFlags.gitLabToken != "" {
			actionableHints = append(actionableHints,
				fmt.Sprintf("The supplied -gitlab-token does not indicate that you have write access to %s.", uploadFlags.repo),
				"Please check the value of the supplied token and its permissions on the code host and try again.",
			)
		} else {
			actionableHints = append(actionableHints,
				fmt.Sprintf("Please retry your request with a -gitlab-token=XXX with with write access to %s.", uploadFlags.repo),
				"This token will be used to check with the code host that the uploading user has write access to the target repository.",
			)
		}
	} else {
		actionableHints = append(actionableHints,
			"Verification is supported for the following code hosts: github.com, gitlab.com.",
			"Please request support for additional code host verification at https://github.com/sourcegraph/sourcegraph/issues/4967.",
		)
	}

	return errorWithHint{err: err, hint: strings.Join(mergeStringSlices(
		[]string{"This Sourcegraph instance has enforced auth for LSIF uploads."},
		actionableHints,
		[]string{"For more details, see https://docs.sourcegraph.com/cli/references/lsif/upload."},
	), "\n")}
}

// emergencyOutput creates a default Output object writing to standard out.
func emergencyOutput() *output.Output {
	return output.NewOutput(os.Stdout, output.OutputOpts{})
}

func mergeStringSlices(ss ...[]string) []string {
	var combined []string
	for _, s := range ss {
		combined = append(combined, s...)
	}

	return combined
}
