package blueprint

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

// ResourceKind identifies the type of resource managed by a blueprint.
type ResourceKind string

const (
	KindDashboard ResourceKind = "dashboard"
	KindMonitor   ResourceKind = "monitor"
	KindInsight   ResourceKind = "insight"
	KindBatchSpec ResourceKind = "batch-spec"
	KindLink      ResourceKind = "link"
)

// ResourceResult captures the outcome of executing a single resource within a blueprint.
type ResourceResult struct {
	Kind      ResourceKind
	Name      string
	Succeeded bool
	Skipped   bool
	Err       error
}

// ExecutionSummary contains the results of executing all resources in a blueprint.
type ExecutionSummary struct {
	Resources []ResourceResult
}

// FailureCount returns the number of resources that failed during execution.
func (s *ExecutionSummary) FailureCount() int {
	count := 0
	for _, r := range s.Resources {
		if r.Err != nil {
			count++
		}
	}
	return count
}

// ExecutorOpts configures an Executor.
type ExecutorOpts struct {
	// Client is used to execute GraphQL requests against Sourcegraph.
	Client api.Client
	// Out is where human-readable progress and summary output will be written.
	// If nil, NewExecutor defaults it to os.Stdout.
	Out io.Writer
	// Vars contains template variables that are passed to GraphQL queries.
	Vars map[string]string
	// DryRun, if true, skips all mutations but still validates and builds variables.
	DryRun bool
	// ContinueOnError, if true, continues executing other resources after a failure.
	ContinueOnError bool
}

// Executor executes a blueprint against a Sourcegraph instance.
type Executor struct {
	client          api.Client
	out             io.Writer
	vars            map[string]string
	dryRun          bool
	continueOnError bool
	currentUserID   *string
	dashboardIDs    map[string]string
	insightIDs      map[string]string
}

// NewExecutor constructs an Executor from options.
func NewExecutor(opts ExecutorOpts) *Executor {
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	return &Executor{
		client:          opts.Client,
		out:             out,
		vars:            opts.Vars,
		dryRun:          opts.DryRun,
		continueOnError: opts.ContinueOnError,
		dashboardIDs:    make(map[string]string),
		insightIDs:      make(map[string]string),
	}
}

// Execute executes all resources defined in the given blueprint.
func (e *Executor) Execute(ctx context.Context, bp *Blueprint, blueprintDir string) (*ExecutionSummary, error) {
	summary := &ExecutionSummary{}

	if len(bp.Dashboards) > 0 {
		fmt.Fprintf(e.out, "\nExecuting dashboard mutations...\n")
		for _, ref := range bp.Dashboards {
			result := e.executeDashboard(ctx, ref, blueprintDir)
			summary.Resources = append(summary.Resources, result)
			e.printResult(result)
			if result.Err != nil && !e.continueOnError {
				return summary, result.Err
			}
		}
	}

	if len(bp.Monitors) > 0 {
		fmt.Fprintf(e.out, "\nExecuting monitor mutations...\n")
		for _, ref := range bp.Monitors {
			result := e.executeMonitor(ctx, ref, blueprintDir)
			summary.Resources = append(summary.Resources, result)
			e.printResult(result)
			if result.Err != nil && !e.continueOnError {
				return summary, result.Err
			}
		}
	}

	if len(bp.Insights) > 0 {
		fmt.Fprintf(e.out, "\nExecuting insight mutations...\n")
		for _, ref := range bp.Insights {
			result := e.executeInsight(ctx, ref, blueprintDir)
			summary.Resources = append(summary.Resources, result)
			e.printResult(result)
			if result.Err != nil && !e.continueOnError {
				return summary, result.Err
			}
		}
	}

	if err := e.linkInsightsToDashboards(ctx, bp.Insights, summary); err != nil && !e.continueOnError {
		return summary, err
	}

	if failed := summary.FailureCount(); failed > 0 {
		return summary, errors.Newf("%d resource(s) failed", failed)
	}

	return summary, nil
}

func (e *Executor) printResult(result ResourceResult) {
	if result.Err != nil {
		fmt.Fprintf(e.out, "  ✗ %s %q: %v\n", result.Kind, result.Name, result.Err)
	} else if result.Skipped {
		fmt.Fprintf(e.out, "  ○ %s %q (skipped - dry run)\n", result.Kind, result.Name)
	} else {
		fmt.Fprintf(e.out, "  ✓ %s %q\n", result.Kind, result.Name)
	}
}

func (e *Executor) executeDashboard(ctx context.Context, ref DashboardRef, blueprintDir string) ResourceResult {
	result := ResourceResult{Kind: KindDashboard, Name: ref.Name}

	query, err := loadGQLFile(ref.Path(blueprintDir))
	if err != nil {
		result.Err = errors.Wrapf(err, "loading %s", ref.Path(blueprintDir))
		return result
	}

	vars, err := e.buildVariables(ctx, query)
	if err != nil {
		result.Err = errors.Wrap(err, "building variables")
		return result
	}

	if e.dryRun {
		result.Skipped = true
		result.Succeeded = true
		return result
	}

	var response struct {
		CreateInsightsDashboard struct {
			Dashboard struct {
				ID string `json:"id"`
			} `json:"dashboard"`
		} `json:"createInsightsDashboard"`
	}

	ok, err := e.client.NewRequest(query, vars).Do(ctx, &response)
	if err != nil {
		result.Err = formatGraphQLError(err, KindDashboard, ref.Name)
		return result
	}
	if !ok {
		result.Err = errors.Newf("executing dashboard mutation %q: no data returned", ref.Name)
		return result
	}

	if id := response.CreateInsightsDashboard.Dashboard.ID; id != "" {
		e.dashboardIDs[ref.Name] = id
	}

	result.Succeeded = true
	return result
}

func (e *Executor) executeMonitor(ctx context.Context, ref MonitorRef, blueprintDir string) ResourceResult {
	result := ResourceResult{Kind: KindMonitor, Name: ref.Name}

	query, err := loadGQLFile(ref.Path(blueprintDir))
	if err != nil {
		result.Err = errors.Wrapf(err, "loading %s", ref.Path(blueprintDir))
		return result
	}

	vars, err := e.buildVariables(ctx, query)
	if err != nil {
		result.Err = errors.Wrap(err, "building variables")
		return result
	}

	if e.dryRun {
		result.Skipped = true
		result.Succeeded = true
		return result
	}

	var response any
	ok, err := e.client.NewRequest(query, vars).Do(ctx, &response)
	if err != nil {
		result.Err = formatGraphQLError(err, KindMonitor, ref.Name)
		return result
	}
	if !ok {
		result.Err = errors.Newf("executing monitor mutation %q: no data returned", ref.Name)
		return result
	}

	result.Succeeded = true
	return result
}

func (e *Executor) executeInsight(ctx context.Context, ref InsightRef, blueprintDir string) ResourceResult {
	result := ResourceResult{Kind: KindInsight, Name: ref.Name}

	query, err := loadGQLFile(ref.Path(blueprintDir))
	if err != nil {
		result.Err = errors.Wrapf(err, "loading %s", ref.Path(blueprintDir))
		return result
	}

	vars, err := e.buildVariables(ctx, query)
	if err != nil {
		result.Err = errors.Wrap(err, "building variables")
		return result
	}

	if e.dryRun {
		result.Skipped = true
		result.Succeeded = true
		return result
	}

	var response struct {
		CreateLineChartSearchInsight struct {
			View struct {
				ID string `json:"id"`
			} `json:"view"`
		} `json:"createLineChartSearchInsight"`
	}

	ok, err := e.client.NewRequest(query, vars).Do(ctx, &response)
	if err != nil {
		result.Err = formatGraphQLError(err, KindInsight, ref.Name)
		return result
	}
	if !ok {
		result.Err = errors.Newf("executing insight mutation %q: no data returned", ref.Name)
		return result
	}

	if id := response.CreateLineChartSearchInsight.View.ID; id != "" {
		e.insightIDs[ref.Name] = id
	}

	result.Succeeded = true
	return result
}

func (e *Executor) linkInsightsToDashboards(ctx context.Context, insights []InsightRef, summary *ExecutionSummary) error {
	var hasLinks bool
	for _, insight := range insights {
		if len(insight.Dashboards) > 0 {
			hasLinks = true
			break
		}
	}
	if !hasLinks || e.dryRun {
		return nil
	}

	fmt.Fprintf(e.out, "\nLinking insights to dashboards...\n")

	const addInsightQuery = `mutation AddInsightViewToDashboard($input: AddInsightViewToDashboardInput!) {
		addInsightViewToDashboard(input: $input) {
			dashboard { id }
		}
	}`

	for _, insight := range insights {
		insightID, ok := e.insightIDs[insight.Name]
		if !ok {
			continue
		}

		for _, dashboardName := range insight.Dashboards {
			dashboardID, ok := e.dashboardIDs[dashboardName]
			if !ok {
				fmt.Fprintf(e.out, "  ⚠ dashboard %q not found for insight %q\n", dashboardName, insight.Name)
				continue
			}

			vars := map[string]any{
				"input": map[string]any{
					"insightViewId": insightID,
					"dashboardId":   dashboardID,
				},
			}

			var response any
			_, err := e.client.NewRequest(addInsightQuery, vars).Do(ctx, &response)
			if err != nil {
				result := ResourceResult{
					Kind: KindLink,
					Name: fmt.Sprintf("%s → %s", insight.Name, dashboardName),
					Err:  errors.Wrapf(err, "linking insight %q to dashboard %q", insight.Name, dashboardName),
				}
				summary.Resources = append(summary.Resources, result)
				fmt.Fprintf(e.out, "  ✗ %s → %s: %v\n", insight.Name, dashboardName, err)
				if !e.continueOnError {
					return result.Err
				}
			} else {
				fmt.Fprintf(e.out, "  ✓ %s → %s\n", insight.Name, dashboardName)
			}
		}
	}

	return nil
}

func loadGQLFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return stripComments(string(data)), nil
}

func stripComments(content string) string {
	var lines []string
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func (e *Executor) buildVariables(ctx context.Context, query string) (map[string]any, error) {
	vars := make(map[string]any)

	for k, v := range e.vars {
		vars[k] = v
	}

	if strings.Contains(query, "$namespaceId") {
		if _, provided := vars["namespaceId"]; !provided {
			if e.dryRun {
				vars["namespaceId"] = "<dry-run-namespace-id>"
			} else {
				nsID, err := e.resolveCurrentUserID(ctx)
				if err != nil {
					return nil, errors.Wrap(err, "resolving namespace")
				}
				vars["namespaceId"] = nsID
			}
		}
	}

	return vars, nil
}

func (e *Executor) resolveCurrentUserID(ctx context.Context) (string, error) {
	if e.currentUserID != nil {
		return *e.currentUserID, nil
	}

	const query = `query { currentUser { id } }`
	var result struct {
		CurrentUser struct {
			ID string `json:"id"`
		} `json:"currentUser"`
	}

	ok, err := e.client.NewQuery(query).Do(ctx, &result)
	if err != nil {
		return "", errors.Wrap(err, "getCurrentUser query failed")
	}
	if !ok {
		return "", errors.New("getCurrentUser: no data returned")
	}
	if result.CurrentUser.ID == "" {
		return "", errors.New("getCurrentUser: not authenticated")
	}

	e.currentUserID = &result.CurrentUser.ID
	return result.CurrentUser.ID, nil
}

func formatGraphQLError(err error, kind ResourceKind, name string) error {
	var gqlErrs api.GraphQlErrors
	if errors.As(err, &gqlErrs) {
		msgs := make([]string, 0, len(gqlErrs))
		for _, ge := range gqlErrs {
			msgs = append(msgs, ge.Error())
		}
		return errors.Newf("%s %q failed: %s", kind, name, strings.Join(msgs, "; "))
	}
	return errors.Wrapf(err, "%s %q failed", kind, name)
}

// PrintExecutionSummary writes a human-readable summary of execution results.
func PrintExecutionSummary(out io.Writer, s *ExecutionSummary, dryRun bool) {
	if s == nil || len(s.Resources) == 0 {
		return
	}

	if dryRun {
		fmt.Fprintf(out, "\nDry run complete. No mutations were executed.\n")
	}

	counts := make(map[ResourceKind]struct{ total, ok, failed int })
	for _, r := range s.Resources {
		c := counts[r.Kind]
		c.total++
		if r.Succeeded {
			c.ok++
		} else if r.Err != nil {
			c.failed++
		}
		counts[r.Kind] = c
	}

	fmt.Fprintf(out, "\nSummary:\n")
	for _, kind := range []ResourceKind{KindDashboard, KindMonitor, KindInsight, KindBatchSpec, KindLink} {
		c := counts[kind]
		if c.total == 0 {
			continue
		}
		fmt.Fprintf(out, "  %s: %d total, %d succeeded, %d failed\n", kind, c.total, c.ok, c.failed)
	}
}
