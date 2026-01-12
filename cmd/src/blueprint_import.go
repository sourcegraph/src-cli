package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/blueprint"
)

type multiStringFlag []string

func (f *multiStringFlag) String() string {
	return strings.Join(*f, ", ")
}

func (f *multiStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (f *multiStringFlag) ToMap() map[string]string {
	result := make(map[string]string)
	for _, v := range *f {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

type blueprintImportOpts struct {
	client          api.Client
	out             io.Writer
	repo            string
	rev             string
	subdir          string
	namespace       string
	vars            map[string]string
	dryRun          bool
	continueOnError bool
}

func init() {
	usage := `
'src blueprint import' imports a blueprint from a Git repository or local directory and executes its resources.

Usage:

    src blueprint import -repo <repository-url-or-path> [flags]

Examples:

    Import a blueprint from the community repository (default):

        $ src blueprint import -subdir monitor/cve-2025-55182

    Import a specific branch or tag:

        $ src blueprint import -rev v1.0.0 -subdir monitor/cve-2025-55182

    Import from a local directory:

        $ src blueprint import -repo ./my-blueprints -subdir monitor/cve-2025-55182

    Import from an absolute path:

        $ src blueprint import -repo /path/to/blueprints

    Import with custom variables:

        $ src blueprint import -subdir monitor/cve-2025-55182 -var webhookUrl=https://example.com/hook

    Dry run to validate without executing:

        $ src blueprint import -subdir monitor/cve-2025-55182 -dry-run

`

	flagSet := flag.NewFlagSet("import", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src blueprint %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		repoFlag        = flagSet.String("repo", defaultBlueprintRepo, "Repository URL (HTTPS) or local path to blueprint")
		revFlag         = flagSet.String("rev", "", "Git revision, branch, or tag to checkout (ignored for local paths)")
		subdirFlag      = flagSet.String("subdir", "", "Subdirectory in repo containing blueprint.yaml")
		namespaceFlag   = flagSet.String("namespace", "", "User or org namespace for mutations (defaults to current user)")
		dryRunFlag      = flagSet.Bool("dry-run", false, "Parse and validate only; do not execute any mutations")
		continueOnError = flagSet.Bool("continue-on-error", false, "Continue applying resources even if one fails")
		varFlags        = multiStringFlag{}
		apiFlags        = api.NewFlags(flagSet)
	)
	flagSet.Var(&varFlags, "var", "Variable in the form key=value; can be repeated")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		opts := blueprintImportOpts{
			client:          client,
			out:             flagSet.Output(),
			repo:            *repoFlag,
			rev:             *revFlag,
			subdir:          *subdirFlag,
			namespace:       *namespaceFlag,
			vars:            varFlags.ToMap(),
			dryRun:          *dryRunFlag,
			continueOnError: *continueOnError,
		}

		return runBlueprintImport(context.Background(), opts)
	}

	blueprintCommands = append(blueprintCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func runBlueprintImport(ctx context.Context, opts blueprintImportOpts) error {
	var src blueprint.BlueprintSource
	var err error

	if opts.subdir == "" {
		src, err = blueprint.ResolveRootSource(opts.repo, opts.rev)
	} else {
		src, err = blueprint.ResolveBlueprintSource(opts.repo, opts.rev, opts.subdir)
	}
	if err != nil {
		return err
	}

	blueprintDir, cleanup, err := src.Prepare(ctx)
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}
	if err != nil {
		return err
	}

	if opts.subdir == "" {
		return runBlueprintImportAll(ctx, opts, blueprintDir)
	}

	return runBlueprintImportSingle(ctx, opts, blueprintDir)
}

func runBlueprintImportAll(ctx context.Context, opts blueprintImportOpts, rootDir string) error {
	found, err := blueprint.FindBlueprints(rootDir)
	if err != nil {
		return err
	}

	if len(found) == 0 {
		fmt.Fprintf(opts.out, "No blueprints found in %s\n", rootDir)
		return nil
	}

	fmt.Fprintf(opts.out, "Found %d blueprint(s) in repository\n\n", len(found))

	exec := blueprint.NewExecutor(blueprint.ExecutorOpts{
		Client:          opts.client,
		Out:             opts.out,
		Vars:            opts.vars,
		DryRun:          opts.dryRun,
		ContinueOnError: opts.continueOnError,
	})

	var lastErr error
	for _, bp := range found {
		subdir, _ := filepath.Rel(rootDir, bp.Dir)
		if subdir == "." {
			subdir = ""
		}

		fmt.Fprintf(opts.out, "--- Importing blueprint: %s", bp.Name)
		if subdir != "" {
			fmt.Fprintf(opts.out, " (%s)", subdir)
		}
		fmt.Fprintf(opts.out, "\n")

		summary, err := exec.Execute(ctx, bp, bp.Dir)
		blueprint.PrintExecutionSummary(opts.out, summary, opts.dryRun)

		if err != nil {
			lastErr = err
			if !opts.continueOnError {
				return err
			}
		}
		fmt.Fprintf(opts.out, "\n")
	}

	return lastErr
}

func runBlueprintImportSingle(ctx context.Context, opts blueprintImportOpts, blueprintDir string) error {
	bp, err := blueprint.Load(blueprintDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.out, "Loaded blueprint: %s\n", bp.Name)
	if bp.Title != "" {
		fmt.Fprintf(opts.out, "  Title: %s\n", bp.Title)
	}
	if len(bp.BatchSpecs) > 0 {
		fmt.Fprintf(opts.out, "  Batch specs: %d\n", len(bp.BatchSpecs))
	}
	if len(bp.Monitors) > 0 {
		fmt.Fprintf(opts.out, "  Monitors: %d\n", len(bp.Monitors))
	}
	if len(bp.Insights) > 0 {
		fmt.Fprintf(opts.out, "  Insights: %d\n", len(bp.Insights))
	}
	if len(bp.Dashboards) > 0 {
		fmt.Fprintf(opts.out, "  Dashboards: %d\n", len(bp.Dashboards))
	}

	exec := blueprint.NewExecutor(blueprint.ExecutorOpts{
		Client:          opts.client,
		Out:             opts.out,
		Vars:            opts.vars,
		DryRun:          opts.dryRun,
		ContinueOnError: opts.continueOnError,
	})

	summary, err := exec.Execute(ctx, bp, blueprintDir)
	blueprint.PrintExecutionSummary(opts.out, summary, opts.dryRun)

	return err
}
