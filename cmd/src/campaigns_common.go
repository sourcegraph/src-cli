package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/neelance/parallel"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/campaigns"
	"github.com/sourcegraph/src-cli/internal/output"
)

var (
	campaignsPendingColor = output.StylePending
	campaignsSuccessColor = output.StyleSuccess
	campaignsSuccessEmoji = output.EmojiSuccess
)

type campaignsApplyFlags struct {
	allowUnsupported bool
	api              *api.Flags
	apply            bool
	cacheDir         string
	tempDir          string
	clearCache       bool
	file             string
	keep             bool
	namespace        string
	parallelism      int
	timeout          time.Duration
}

func newCampaignsApplyFlags(flagSet *flag.FlagSet, cacheDir, tempDir string) *campaignsApplyFlags {
	caf := &campaignsApplyFlags{
		api: api.NewFlags(flagSet),
	}

	flagSet.BoolVar(
		&caf.allowUnsupported, "allow-unsupported", false,
		"Allow unsupported code hosts.",
	)
	flagSet.BoolVar(
		&caf.apply, "apply", false,
		"Ignored.",
	)
	flagSet.StringVar(
		&caf.cacheDir, "cache", cacheDir,
		"Directory for caching results.",
	)
	flagSet.BoolVar(
		&caf.clearCache, "clear-cache", false,
		"If true, clears the cache and executes all steps anew.",
	)
	flagSet.StringVar(
		&caf.tempDir, "tmp", tempDir,
		"Directory for storing temporary data, such as repository archives when executing campaign specs or log files. Default is /tmp. Can also be set with environment variable SRC_CAMPAIGNS_TMP_DIR; if both are set, this flag will be used and not the environment variable.",
	)
	flagSet.StringVar(
		&caf.file, "f", "",
		"The campaign spec file to read.",
	)
	flagSet.BoolVar(
		&caf.keep, "keep-logs", false,
		"Retain logs after executing steps.",
	)
	flagSet.StringVar(
		&caf.namespace, "namespace", "",
		"The user or organization namespace to place the campaign within.",
	)
	flagSet.StringVar(&caf.namespace, "n", "", "Alias for -namespace.")

	flagSet.IntVar(
		&caf.parallelism, "j", 0,
		"The maximum number of parallel jobs. (Default: GOMAXPROCS.)",
	)
	flagSet.DurationVar(
		&caf.timeout, "timeout", 60*time.Minute,
		"The maximum duration a single set of campaign steps can take.",
	)

	return caf
}

func campaignsCreatePending(out *output.Output, message string) output.Pending {
	return out.Pending(output.Line("", campaignsPendingColor, message))
}

func campaignsCompletePending(p output.Pending, message string) {
	p.Complete(output.Line(campaignsSuccessEmoji, campaignsSuccessColor, message))
}

func campaignsDefaultCacheDir() string {
	uc, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	return path.Join(uc, "sourcegraph", "campaigns")
}

// campaignsDefaultTempDirPrefix returns the prefix to be passed to ioutil.TempFile. If the
// environment variable SRC_CAMPAIGNS_TMP_DIR is set, that is used as the
// prefix. Otherwise we use "/tmp".
func campaignsDefaultTempDirPrefix() string {
	p := os.Getenv("SRC_CAMPAIGNS_TMP_DIR")
	if p != "" {
		return p
	}
	// On macOS, we use an explicit prefix for our temp directories, because
	// otherwise Go would use $TMPDIR, which is set to `/var/folders` per
	// default on macOS. But Docker for Mac doesn't have `/var/folders` in its
	// default set of shared folders, but it does have `/tmp` in there.
	if runtime.GOOS == "darwin" {
		return "/tmp"

	}
	return os.TempDir()
}

func campaignsOpenFileFlag(flag *string) (io.ReadCloser, error) {
	if flag == nil || *flag == "" || *flag == "-" {
		return os.Stdin, nil
	}

	file, err := os.Open(*flag)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file %q", *flag)
	}
	return file, nil
}

// campaignsExecute performs all the steps required to upload the campaign spec
// to Sourcegraph, including execution as needed. The return values are the
// spec ID, spec URL, and error.
func campaignsExecute(ctx context.Context, out *output.Output, svc *campaigns.Service, flags *campaignsApplyFlags) (campaigns.CampaignSpecID, string, error) {
	// Parse flags and build up our service options.
	var errs *multierror.Error

	specFile, err := campaignsOpenFileFlag(&flags.file)
	if err != nil {
		errs = multierror.Append(errs, err)
	} else {
		defer specFile.Close()
	}

	if flags.namespace == "" {
		errs = multierror.Append(errs, &usageError{errors.New("a namespace must be provided with -namespace")})
	}

	opts := campaigns.ExecutorOpts{
		Cache:      svc.NewExecutionCache(flags.cacheDir),
		ClearCache: flags.clearCache,
		KeepLogs:   flags.keep,
		Timeout:    flags.timeout,
		TempDir:    flags.tempDir,
	}
	if flags.parallelism <= 0 {
		opts.Parallelism = runtime.GOMAXPROCS(0)
	} else {
		opts.Parallelism = flags.parallelism
	}
	executor := svc.NewExecutor(opts, nil)

	if errs != nil {
		return "", "", errs
	}

	pending := campaignsCreatePending(out, "Parsing campaign spec")
	campaignSpec, rawSpec, err := svc.ParseCampaignSpec(specFile)
	if err != nil {
		return "", "", errors.Wrap(err, "parsing campaign spec")
	}

	if err := campaignsValidateSpec(out, campaignSpec); err != nil {
		return "", "", err
	}
	campaignsCompletePending(pending, "Parsing campaign spec")

	pending = campaignsCreatePending(out, "Resolving namespace")
	namespace, err := svc.ResolveNamespace(ctx, flags.namespace)
	if err != nil {
		return "", "", err
	}
	campaignsCompletePending(pending, "Resolving namespace")

	imageProgress := out.Progress([]output.ProgressBar{{
		Label: "Preparing container images",
		Max:   float64(len(campaignSpec.Steps)),
	}}, nil)
	err = svc.SetDockerImages(ctx, campaignSpec, func(step int) {
		imageProgress.SetValue(0, float64(step))
	})
	if err != nil {
		return "", "", err
	}
	imageProgress.Complete()

	pending = campaignsCreatePending(out, "Resolving repositories")
	repos, err := svc.ResolveRepositories(ctx, campaignSpec)
	if err != nil {
		if repoSet, ok := err.(campaigns.UnsupportedRepoSet); ok {
			campaignsCompletePending(pending, "Resolved repositories.")

			block := out.Block(output.Line(" ", output.StyleWarning, "Some repositories are hosted on unsupported code hosts and will be skipped. Use the -allow-unsupported flag to avoid skipping them."))
			for repo := range repoSet {
				block.Write(repo.Name)
			}
			block.Close()
		} else {
			return "", "", errors.Wrap(err, "resolving repositories")
		}
	} else {
		campaignsCompletePending(pending, "Resolved repositories.")
	}

	var progress output.Progress
	var maxRepoName int
	completed := map[string]bool{}
	specs, err := svc.ExecuteCampaignSpec(ctx, repos, executor, campaignSpec, func(statuses []*campaigns.TaskStatus) {
		if progress == nil {
			progress = out.Progress([]output.ProgressBar{{
				Label: fmt.Sprintf("Executing steps in %d repositories", len(statuses)),
				Max:   float64(len(statuses)),
			}}, nil)
		}

		unloggedCompleted := []*campaigns.TaskStatus{}

		for _, ts := range statuses {
			if len(ts.RepoName) > maxRepoName {
				maxRepoName = len(ts.RepoName)
			}

			if ts.FinishedAt.IsZero() {
				continue
			}

			if !completed[ts.RepoName] {
				completed[ts.RepoName] = true
				unloggedCompleted = append(unloggedCompleted, ts)
			}

		}

		progress.SetValue(0, float64(len(completed)))

		for _, ts := range unloggedCompleted {
			var statusText string

			if ts.ChangesetSpec == nil {
				statusText = "No changes"
			} else {
				fileDiffs, err := diff.ParseMultiFileDiff([]byte(ts.ChangesetSpec.Commits[0].Diff))
				if err != nil {
					panic(err)
				}

				statusText = diffStatDescription(fileDiffs) + " " + diffStatDiagram(sumDiffStats(fileDiffs))
			}

			if ts.Cached {
				statusText += " (cached)"
			}

			progress.Verbosef("%-*s %s", maxRepoName, ts.RepoName, statusText)
		}
	})

	if err != nil {
		return "", "", err
	}
	if progress != nil {
		progress.Complete()
	}

	if logFiles := executor.LogFiles(); len(logFiles) > 0 && flags.keep {
		func() {
			block := out.Block(output.Line("", campaignsSuccessColor, "Preserving log files:"))
			defer block.Close()

			for _, file := range logFiles {
				block.Write(file)
			}
		}()
	}

	progress = out.Progress([]output.ProgressBar{
		{Label: "Sending changeset specs", Max: float64(len(specs))},
	}, nil)
	ids := make([]campaigns.ChangesetSpecID, len(specs))
	for i, spec := range specs {
		id, err := svc.CreateChangesetSpec(ctx, spec)
		if err != nil {
			return "", "", err
		}
		ids[i] = id
		progress.SetValue(0, float64(i+1))
	}
	progress.Complete()

	pending = campaignsCreatePending(out, "Creating campaign spec on Sourcegraph")
	id, url, err := svc.CreateCampaignSpec(ctx, namespace, rawSpec, ids)
	if err != nil {
		return "", "", err
	}
	campaignsCompletePending(pending, "Creating campaign spec on Sourcegraph")

	return id, url, nil
}

// printExecutionError is used to print the possible error returned by
// campaignsExecute.
func printExecutionError(out *output.Output, err error) {
	out.Write("")

	writeErr := func(block *output.Block, err error) {
		if block == nil {
			return
		}

		if taskErr, ok := err.(campaigns.TaskExecutionErr); ok {
			block.Write(formatTaskExecutionErr(taskErr))
		} else {
			block.Write(err.Error())
		}
	}

	var block *output.Block
	singleErrHeader := output.Line(output.EmojiFailure, output.StyleWarning, "Error:")

	if parErr, ok := err.(parallel.Errors); ok {
		if len(parErr) > 1 {
			block = out.Block(output.Linef(output.EmojiFailure, output.StyleWarning, "%d errors:", len(parErr)))
		} else {
			block = out.Block(singleErrHeader)
		}

		for _, e := range parErr {
			writeErr(block, e)
		}
	} else {
		block = out.Block(singleErrHeader)
		writeErr(block, err)
	}

	if block != nil {
		block.Close()
	}
	out.Write("")
}

func formatTaskExecutionErr(err campaigns.TaskExecutionErr) string {
	return fmt.Sprintf(
		"%s%s%s:\n%s\nLog: %s\n",
		output.StyleBold,
		err.Repository,
		output.StyleReset,
		err.Err,
		err.Logfile,
	)
}

func sumDiffStats(fileDiffs []*diff.FileDiff) diff.Stat {
	sum := diff.Stat{}
	for _, fileDiff := range fileDiffs {
		stat := fileDiff.Stat()
		sum.Added += stat.Added
		sum.Changed += stat.Changed
		sum.Deleted += stat.Deleted
	}
	return sum
}

func diffStatDescription(fileDiffs []*diff.FileDiff) string {
	var plural string
	if len(fileDiffs) > 1 {
		plural = "s"
	}

	return fmt.Sprintf("%d file%s changed", len(fileDiffs), plural)
}

func diffStatDiagram(stat diff.Stat) string {
	const maxWidth = 20
	added := float64(stat.Added + stat.Changed)
	deleted := float64(stat.Deleted + stat.Changed)
	if total := added + deleted; total > maxWidth {
		x := float64(20) / total
		added *= x
		deleted *= x
	}
	return fmt.Sprintf("%s%s%s%s",
		output.StyleLinesAdded, strings.Repeat("+", int(added)),
		output.StyleLinesDeleted, strings.Repeat("-", int(deleted)),
	)
}
