package main

import (
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/sourcegraph/scip/bindings/go/scip"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/codeintel"
)

var codeintelUploadFlags struct {
	file           string
	gzipCompressed bool

	// UploadRecordOptions
	repo              string
	commit            string
	root              string
	indexer           string
	indexerVersion    string
	associatedIndexID int

	// SourcegraphInstanceOptions
	uploadRoute      string
	maxPayloadSizeMb int64
	maxConcurrency   int

	// Codehost authorization secrets
	gitHubToken string
	gitLabToken string

	// Output and error behavior
	ignoreUploadFailures bool
	noProgress           bool
	verbosity            int
	json                 bool
	open                 bool
	apiFlags             *api.Flags
}

var (
	codeintelUploadFlagSet = flag.NewFlagSet("upload", flag.ExitOnError)
	apiClientFlagSet       = flag.NewFlagSet("upload client", flag.ExitOnError)
	// Used to include the insecure-skip-verify flag in the help output, as we don't use any of the
	// other api.Client methods, so only the insecureSkipVerify flag is relevant here.
	dummyflag bool
)

func init() {
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.file, "file", "", `The path to the SCIP index file.`)

	// UploadRecordOptions
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.repo, "repo", "", `The name of the repository (e.g. github.com/gorilla/mux). By default, derived from the origin remote.`)
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.commit, "commit", "", `The 40-character hash of the commit. Defaults to the currently checked-out commit.`)
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.root, "root", "", `The path in the repository that matches the SCIP projectRoot (e.g. cmd/project1). Defaults to the directory where the SCIP index file is located.`)
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.indexer, "indexer", "", `The name of the indexer that generated the dump. This will override the 'toolInfo.name' field in the metadata section of SCIP index. This must be supplied if the indexer does not set this field (in which case the upload will fail with an explicit message).`)
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.indexerVersion, "indexerVersion", "", `The version of the indexer that generated the dump. This will override the 'toolInfo.version' field in the metadata section of SCIP index. This must be supplied if the indexer does not set this field (in which case the upload will fail with an explicit message).`)
	codeintelUploadFlagSet.IntVar(&codeintelUploadFlags.associatedIndexID, "associated-index-id", -1, "ID of the associated index record for this upload. For internal use only.")

	// SourcegraphInstanceOptions
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.uploadRoute, "upload-route", "/.api/scip/upload", "The path of the upload route. For internal use only.")
	codeintelUploadFlagSet.Int64Var(&codeintelUploadFlags.maxPayloadSizeMb, "max-payload-size", 100, `The maximum upload size (in megabytes). Indexes exceeding this limit will be uploaded over multiple HTTP requests.`)
	codeintelUploadFlagSet.IntVar(&codeintelUploadFlags.maxConcurrency, "max-concurrency", -1, "The maximum number of concurrent uploads. Only relevant for multipart uploads. Defaults to all parts concurrently.")

	// Codehost authorization secrets
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.gitHubToken, "github-token", "", `A GitHub access token with 'public_repo' scope that Sourcegraph uses to verify you have access to the repository.`)
	codeintelUploadFlagSet.StringVar(&codeintelUploadFlags.gitLabToken, "gitlab-token", "", `A GitLab access token with 'read_api' scope that Sourcegraph uses to verify you have access to the repository.`)

	// Output and error behavior
	codeintelUploadFlagSet.BoolVar(&codeintelUploadFlags.ignoreUploadFailures, "ignore-upload-failure", false, `Exit with status code zero on upload failure.`)
	codeintelUploadFlagSet.BoolVar(&codeintelUploadFlags.noProgress, "no-progress", false, `Do not display progress updates.`)
	codeintelUploadFlagSet.IntVar(&codeintelUploadFlags.verbosity, "trace", 0, "-trace=0 shows no logs; -trace=1 shows requests and response metadata; -trace=2 shows headers, -trace=3 shows response body")
	codeintelUploadFlagSet.BoolVar(&codeintelUploadFlags.json, "json", false, `Output relevant state in JSON on success.`)
	codeintelUploadFlagSet.BoolVar(&codeintelUploadFlags.open, "open", false, `Open the SCIP upload page in your browser.`)
	codeintelUploadFlagSet.BoolVar(&dummyflag, "insecure-skip-verify", false, "Skip validation of TLS certificates against trusted chains")
}

// parseAndValidateCodeIntelUploadFlags calls codeintelUploadFlagset.Parse, then infers values for
// missing flags, normalizes supplied values, and validates the state of the codeintelUploadFlags
// object.
//
// On success, the global codeintelUploadFlags object will be populated with valid values. An
// error is returned on failure.
func parseAndValidateCodeIntelUploadFlags(args []string) (*output.Output, error) {
	if err := codeintelUploadFlagSet.Parse(args); err != nil {
		return nil, err
	}

	out := codeintelUploadOutput()

	// extract only the -insecure-skip-verify flag so we dont get 'flag provided but not defined'
	var insecureSkipVerifyFlag []string
	for _, s := range args {
		if strings.HasPrefix(s, "-insecure-skip-verify") {
			insecureSkipVerifyFlag = append(insecureSkipVerifyFlag, s)
		}
	}

	// parse the api client flags separately and then populate the codeintelUploadFlags struct with the result
	// we could just use insecureSkipVerify but I'm including everything here because it costs nothing
	// and maybe we'll use some in the future
	codeintelUploadFlags.apiFlags = api.NewFlags(apiClientFlagSet)
	if err := apiClientFlagSet.Parse(insecureSkipVerifyFlag); err != nil {
		return nil, err
	}

	if !isFlagSet(codeintelUploadFlagSet, "file") {
		defaultFile, err := inferDefaultFile(out)
		if err != nil {
			return nil, err
		}
		codeintelUploadFlags.file = defaultFile
	}

	// Check to see if input file exists
	if _, err := os.Stat(codeintelUploadFlags.file); os.IsNotExist(err) {
		if !isFlagSet(codeintelUploadFlagSet, "file") {
			return nil, formatInferenceError(argumentInferenceError{"file", err})
		}

		return nil, errors.Newf("file %q does not exist", codeintelUploadFlags.file)
	}

	// Check for new file existence after transformation
	if _, err := os.Stat(codeintelUploadFlags.file); os.IsNotExist(err) {
		return nil, errors.Newf("file %q does not exist", codeintelUploadFlags.file)
	}

	if err := inferGzipFlag(); err != nil {
		return nil, err
	}

	// Infer the remaining default arguments (may require reading from new file)
	if inferenceErrors := inferMissingCodeIntelUploadFlags(); len(inferenceErrors) > 0 {
		return nil, formatInferenceError(inferenceErrors[0])
	}

	if err := validateCodeIntelUploadFlags(); err != nil {
		return nil, err
	}

	return out, nil
}

func inferGzipFlag() error {
	if codeintelUploadFlags.gzipCompressed || path.Ext(codeintelUploadFlags.file) == ".gz" {
		file, err := os.Open(codeintelUploadFlags.file)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := checkGzipHeader(file); err != nil {
			return errors.Wrapf(err, "could not verify that %s is a valid gzip file", codeintelUploadFlags.file)
		}
		codeintelUploadFlags.gzipCompressed = true
	}

	return nil
}

// codeintelUploadOutput returns an output object that should be used to print the progres
// of requests made during this upload. If -json, -no-progress, or -trace>0 is given,
// then no output object is defined.
//
// For -no-progress and -trace>0 conditions, emergency loggers will be used to display
// inferred arguments and the URL at which processing status is shown.
func codeintelUploadOutput() (out *output.Output) {
	if codeintelUploadFlags.json || codeintelUploadFlags.noProgress || codeintelUploadFlags.verbosity > 0 {
		return nil
	}

	return output.NewOutput(flag.CommandLine.Output(), output.OutputOpts{
		Verbose: true,
	})
}

type argumentInferenceError struct {
	argument string
	err      error
}

func inferDefaultFile(out *output.Output) (string, error) {
	const scipFilename = "index.scip"
	const scipCompressedFilename = "index.scip.gz"

	hasSCIP, err := doesFileExist(scipFilename)
	if err != nil {
		return "", err
	}
	hasCompressedSCIP, err := doesFileExist(scipCompressedFilename)
	if err != nil {
		return "", err
	}

	if !hasSCIP && !hasCompressedSCIP {
		return "", formatInferenceError(argumentInferenceError{"file", errors.Newf("Unable to locate SCIP index. Checked paths: %q and %q.", scipFilename, scipCompressedFilename)})
	} else if hasCompressedSCIP {
		if hasSCIP {
			out.WriteLine(output.Linef(output.EmojiInfo, output.StyleBold, "Both %s and %s exist, choosing %s", scipFilename, scipCompressedFilename, scipCompressedFilename))
		}
		return scipCompressedFilename, nil
	} else {
		return scipFilename, nil
	}
}

func doesFileExist(filename string) (bool, error) {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return !info.IsDir(), nil
	}
}

func formatInferenceError(inferenceErr argumentInferenceError) error {
	return errorWithHint{
		err: inferenceErr.err, hint: strings.Join([]string{
			fmt.Sprintf(
				"Unable to determine %s from environment. Check your working directory or supply -%s={value} explicitly",
				inferenceErr.argument,
				inferenceErr.argument,
			),
		}, "\n"),
	}
}

// inferMissingCodeIntelUploadFlags updates the flags values which were not explicitly
// supplied by the user with default values inferred from the current git state and
// filesystem.
//
// Note: This function must not be called before codeintelUploadFlagset.Parse.
func inferMissingCodeIntelUploadFlags() (inferErrors []argumentInferenceError) {
	indexerName, indexerVersion, readIndexerNameAndVersionErr := readIndexerNameAndVersion(codeintelUploadFlags.file)
	getIndexerName := func() (string, error) { return indexerName, readIndexerNameAndVersionErr }
	getIndexerVersion := func() (string, error) { return indexerVersion, readIndexerNameAndVersionErr }

	if err := inferUnsetFlag("repo", &codeintelUploadFlags.repo, codeintel.InferRepo); err != nil {
		inferErrors = append(inferErrors, *err)
	}
	if err := inferUnsetFlag("commit", &codeintelUploadFlags.commit, codeintel.InferCommit); err != nil {
		inferErrors = append(inferErrors, *err)
	}
	if err := inferUnsetFlag("root", &codeintelUploadFlags.root, inferIndexRoot); err != nil {
		inferErrors = append(inferErrors, *err)
	}
	if err := inferUnsetFlag("indexer", &codeintelUploadFlags.indexer, getIndexerName); err != nil {
		inferErrors = append(inferErrors, *err)
	}
	if err := inferUnsetFlag("indexerVersion", &codeintelUploadFlags.indexerVersion, getIndexerVersion); err != nil {
		inferErrors = append(inferErrors, *err)
	}

	return inferErrors
}

// inferUnsetFlag conditionally updates the value of the given pointer with the
// return value of the given function. If the flag with the given name was supplied
// by the user, then this function no-ops. An argumentInferenceError is returned if
// the given function returns an error.
//
// Note: This function must not be called before codeintelUploadFlagset.Parse.
func inferUnsetFlag(name string, target *string, f func() (string, error)) *argumentInferenceError {
	if isFlagSet(codeintelUploadFlagSet, name) {
		return nil
	}

	value, err := f()
	if err != nil {
		return &argumentInferenceError{name, err}
	}

	*target = value
	return nil
}

// isFlagSet returns true if the flag with the given name was supplied by the user.
// This lets us distinguish between zero-values (empty strings) and void values without
// requiring pointers and adding a layer of indirection deeper in the program.
func isFlagSet(fs *flag.FlagSet, name string) (found bool) {
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})

	return found
}

// inferIndexRoot returns the root directory based on the configured index file path.
//
// Note: This function must not be called before codeintelUploadFlagset.Parse.
func inferIndexRoot() (string, error) {
	return codeintel.InferRoot(codeintelUploadFlags.file)
}

// readIndexerNameAndVersion returns the indexer name and version values read from the
// toolInfo value in the configured index file.
//
// Note: This function must not be called before codeintelUploadFlagset.Parse.
func readIndexerNameAndVersion(indexFile string) (string, string, error) {
	file, err := os.Open(indexFile)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	var indexReader io.Reader = file

	if codeintelUploadFlags.gzipCompressed {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return "", "", err
		}
		indexReader = gzipReader
		defer gzipReader.Close()
	}

	var metadata *scip.Metadata

	visitor := scip.IndexVisitor{
		VisitMetadata: func(ctx context.Context, m *scip.Metadata) error {
			metadata = m
			return nil
		},
	}

	// convert file to io.Reader
	if err := visitor.ParseStreaming(context.Background(), indexReader); err != nil {
		return "", "", err
	}

	if metadata == nil || metadata.ToolInfo == nil {
		return "", "", errors.New("index file does not contain valid metadata")
	}

	return metadata.ToolInfo.Name, metadata.ToolInfo.Version, nil
}

// validateCodeIntelUploadFlags returns an error if any of the parsed flag values are illegal.
//
// Note: This function must not be called before codeintelUploadFlagset.Parse.
func validateCodeIntelUploadFlags() error {
	codeintelUploadFlags.root = codeintel.SanitizeRoot(codeintelUploadFlags.root)

	if strings.HasPrefix(codeintelUploadFlags.root, "..") {
		return errors.New("root must not be outside of repository")
	}

	if codeintelUploadFlags.maxPayloadSizeMb < 25 {
		return errors.New("max-payload-size must be at least 25 (MB)")
	}

	return nil
}

func checkGzipHeader(r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	return nil
}
