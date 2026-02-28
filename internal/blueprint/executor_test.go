package blueprint

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcegraph/src-cli/internal/api"
)

type mockRequest struct {
	query    string
	vars     map[string]any
	response any
	ok       bool
	err      error
}

func (r *mockRequest) Do(ctx context.Context, result any) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	if r.response != nil {
		copyResponse(r.response, result)
	}
	return r.ok, nil
}

func (r *mockRequest) DoRaw(ctx context.Context, result any) (bool, error) {
	return r.Do(ctx, result)
}

type mockClient struct {
	requests      []*mockRequest
	responses     map[string]mockResponse
	callCount     int
	lastQuery     string
	lastVars      map[string]any
	currentUserID string
}

type mockResponse struct {
	response any
	ok       bool
	err      error
}

func (c *mockClient) NewQuery(query string) api.Request {
	return c.NewRequest(query, nil)
}

func (c *mockClient) NewRequest(query string, vars map[string]any) api.Request {
	c.lastQuery = query
	c.lastVars = vars
	c.callCount++

	if strings.Contains(query, "currentUser") {
		return &mockRequest{
			query: query,
			vars:  vars,
			response: map[string]any{
				"currentUser": map[string]any{
					"id": c.currentUserID,
				},
			},
			ok: true,
		}
	}

	for pattern, resp := range c.responses {
		if strings.Contains(query, pattern) {
			return &mockRequest{
				query:    query,
				vars:     vars,
				response: resp.response,
				ok:       resp.ok,
				err:      resp.err,
			}
		}
	}

	if c.callCount <= len(c.requests) {
		req := c.requests[c.callCount-1]
		req.query = query
		req.vars = vars
		return req
	}

	return &mockRequest{query: query, vars: vars, ok: true}
}

func (c *mockClient) NewHTTPRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	return nil, nil
}

func (c *mockClient) Do(req *http.Request) (*http.Response, error) {
	return nil, nil
}

func copyResponse(src, dst any) {
	switch d := dst.(type) {
	case *struct {
		CreateInsightsDashboard struct {
			Dashboard struct {
				ID string `json:"id"`
			} `json:"dashboard"`
		} `json:"createInsightsDashboard"`
	}:
		if m, ok := src.(map[string]any); ok {
			if cid, ok := m["createInsightsDashboard"].(map[string]any); ok {
				if dash, ok := cid["dashboard"].(map[string]any); ok {
					if id, ok := dash["id"].(string); ok {
						d.CreateInsightsDashboard.Dashboard.ID = id
					}
				}
			}
		}
	case *struct {
		CreateLineChartSearchInsight struct {
			View struct {
				ID string `json:"id"`
			} `json:"view"`
		} `json:"createLineChartSearchInsight"`
	}:
		if m, ok := src.(map[string]any); ok {
			if cli, ok := m["createLineChartSearchInsight"].(map[string]any); ok {
				if view, ok := cli["view"].(map[string]any); ok {
					if id, ok := view["id"].(string); ok {
						d.CreateLineChartSearchInsight.View.ID = id
					}
				}
			}
		}
	}
}

func setupTestBlueprint(t *testing.T) (string, *Blueprint) {
	t.Helper()
	dir := t.TempDir()

	resourcesDir := filepath.Join(dir, "resources")
	for _, subdir := range []string{"dashboards", "monitors", "insights"} {
		if err := os.MkdirAll(filepath.Join(resourcesDir, subdir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dashboardGQL := `mutation CreateDashboard($input: CreateInsightsDashboardInput!) {
		createInsightsDashboard(input: $input) {
			dashboard { id }
		}
	}`
	if err := os.WriteFile(filepath.Join(resourcesDir, "dashboards", "test-dashboard.gql"), []byte(dashboardGQL), 0o644); err != nil {
		t.Fatal(err)
	}

	monitorGQL := `mutation CreateMonitor($input: MonitorInput!) {
		createCodeMonitor(input: $input) { id }
	}`
	if err := os.WriteFile(filepath.Join(resourcesDir, "monitors", "test-monitor.gql"), []byte(monitorGQL), 0o644); err != nil {
		t.Fatal(err)
	}

	insightGQL := `mutation CreateInsight($input: LineChartSearchInsightInput!) {
		createLineChartSearchInsight(input: $input) {
			view { id }
		}
	}`
	if err := os.WriteFile(filepath.Join(resourcesDir, "insights", "test-insight.gql"), []byte(insightGQL), 0o644); err != nil {
		t.Fatal(err)
	}

	bp := &Blueprint{
		Version: 1,
		Name:    "test-blueprint",
		Dashboards: []DashboardRef{
			{Name: "test-dashboard"},
		},
		Monitors: []MonitorRef{
			{Name: "test-monitor"},
		},
		Insights: []InsightRef{
			{Name: "test-insight", Dashboards: []string{"test-dashboard"}},
		},
	}

	return dir, bp
}

func TestExecutor_DryRun(t *testing.T) {
	dir, bp := setupTestBlueprint(t)
	out := &bytes.Buffer{}

	client := &mockClient{currentUserID: "user-123"}
	executor := NewExecutor(ExecutorOpts{
		Client: client,
		Out:    out,
		DryRun: true,
	})

	summary, err := executor.Execute(context.Background(), bp, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.callCount != 0 {
		t.Errorf("expected 0 API calls in dry-run mode, got %d", client.callCount)
	}

	for _, r := range summary.Resources {
		if !r.Skipped {
			t.Errorf("resource %s %q should be skipped in dry-run", r.Kind, r.Name)
		}
		if !r.Succeeded {
			t.Errorf("resource %s %q should succeed in dry-run", r.Kind, r.Name)
		}
	}

	if !strings.Contains(out.String(), "skipped - dry run") {
		t.Errorf("output should mention dry run: %s", out.String())
	}
}

func TestExecutor_Success(t *testing.T) {
	dir, bp := setupTestBlueprint(t)
	out := &bytes.Buffer{}

	client := &mockClient{
		currentUserID: "user-123",
		responses: map[string]mockResponse{
			"createInsightsDashboard": {
				response: map[string]any{
					"createInsightsDashboard": map[string]any{
						"dashboard": map[string]any{"id": "dash-id-1"},
					},
				},
				ok: true,
			},
			"createCodeMonitor": {
				response: map[string]any{"createCodeMonitor": map[string]any{"id": "mon-id-1"}},
				ok:       true,
			},
			"createLineChartSearchInsight": {
				response: map[string]any{
					"createLineChartSearchInsight": map[string]any{
						"view": map[string]any{"id": "insight-id-1"},
					},
				},
				ok: true,
			},
			"addInsightViewToDashboard": {
				response: map[string]any{},
				ok:       true,
			},
		},
	}

	executor := NewExecutor(ExecutorOpts{
		Client: client,
		Out:    out,
	})

	summary, err := executor.Execute(context.Background(), bp, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.FailureCount() != 0 {
		t.Errorf("expected 0 failures, got %d", summary.FailureCount())
	}

	dashboardCount := 0
	monitorCount := 0
	insightCount := 0
	for _, r := range summary.Resources {
		switch r.Kind {
		case KindDashboard:
			dashboardCount++
		case KindMonitor:
			monitorCount++
		case KindInsight:
			insightCount++
		}
		if !r.Succeeded {
			t.Errorf("resource %s %q should have succeeded", r.Kind, r.Name)
		}
	}

	if dashboardCount != 1 {
		t.Errorf("expected 1 dashboard, got %d", dashboardCount)
	}
	if monitorCount != 1 {
		t.Errorf("expected 1 monitor, got %d", monitorCount)
	}
	if insightCount != 1 {
		t.Errorf("expected 1 insight, got %d", insightCount)
	}
}

func TestExecutor_ContinueOnError(t *testing.T) {
	dir, bp := setupTestBlueprint(t)
	out := &bytes.Buffer{}

	client := &mockClient{
		currentUserID: "user-123",
		responses: map[string]mockResponse{
			"createInsightsDashboard": {
				err: api.GraphQlErrors{&api.GraphQlError{}},
			},
			"createCodeMonitor": {
				response: map[string]any{"createCodeMonitor": map[string]any{"id": "mon-id-1"}},
				ok:       true,
			},
			"createLineChartSearchInsight": {
				response: map[string]any{
					"createLineChartSearchInsight": map[string]any{
						"view": map[string]any{"id": "insight-id-1"},
					},
				},
				ok: true,
			},
		},
	}

	executor := NewExecutor(ExecutorOpts{
		Client:          client,
		Out:             out,
		ContinueOnError: true,
	})

	summary, err := executor.Execute(context.Background(), bp, dir)
	if err == nil {
		t.Fatal("expected error due to failed dashboard")
	}

	if summary.FailureCount() != 1 {
		t.Errorf("expected 1 failure, got %d", summary.FailureCount())
	}

	if len(summary.Resources) != 3 {
		t.Errorf("expected 3 resources (continued after error), got %d", len(summary.Resources))
	}
}

func TestExecutor_StopOnError(t *testing.T) {
	dir, bp := setupTestBlueprint(t)
	out := &bytes.Buffer{}

	client := &mockClient{
		currentUserID: "user-123",
		responses: map[string]mockResponse{
			"createInsightsDashboard": {
				err: api.GraphQlErrors{&api.GraphQlError{}},
			},
		},
	}

	executor := NewExecutor(ExecutorOpts{
		Client:          client,
		Out:             out,
		ContinueOnError: false,
	})

	summary, err := executor.Execute(context.Background(), bp, dir)
	if err == nil {
		t.Fatal("expected error")
	}

	if len(summary.Resources) != 1 {
		t.Errorf("expected 1 resource (stopped after error), got %d", len(summary.Resources))
	}
}

func TestExecutor_MissingGQLFile(t *testing.T) {
	dir := t.TempDir()
	bp := &Blueprint{
		Version:    1,
		Name:       "test",
		Dashboards: []DashboardRef{{Name: "nonexistent"}},
	}

	client := &mockClient{currentUserID: "user-123"}
	executor := NewExecutor(ExecutorOpts{
		Client: client,
		Out:    &bytes.Buffer{},
	})

	summary, err := executor.Execute(context.Background(), bp, dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}

	if len(summary.Resources) != 1 {
		t.Fatalf("expected 1 resource result, got %d", len(summary.Resources))
	}
	if summary.Resources[0].Err == nil {
		t.Error("expected error in resource result")
	}
}

func TestStripComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no comments",
			input: "query { user { id } }",
			want:  "query { user { id } }",
		},
		{
			name:  "line comment removed",
			input: "# comment\nquery { user { id } }",
			want:  "query { user { id } }",
		},
		{
			name:  "multiple comments removed",
			input: "# first\n# second\nquery { user { id } }\n# trailing",
			want:  "query { user { id } }",
		},
		{
			name:  "indented comment removed",
			input: "query {\n  # comment\n  user { id }\n}",
			want:  "query {\n  user { id }\n}",
		},
		{
			name:  "preserves non-comment lines",
			input: "mutation {\n  test\n}",
			want:  "mutation {\n  test\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripComments(tt.input)
			if got != tt.want {
				t.Errorf("stripComments() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadGQLFile(t *testing.T) {
	dir := t.TempDir()
	content := "# comment\nmutation Test { test }"
	path := filepath.Join(dir, "test.gql")

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadGQLFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(got, "# comment") {
		t.Errorf("comments should be stripped: %q", got)
	}
	if !strings.Contains(got, "mutation Test") {
		t.Errorf("query content should be preserved: %q", got)
	}
}

func TestLoadGQLFile_NotFound(t *testing.T) {
	_, err := loadGQLFile("/nonexistent/path.gql")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFormatGraphQLError(t *testing.T) {
	t.Run("GraphQL errors", func(t *testing.T) {
		gqlErr := api.GraphQlErrors{&api.GraphQlError{}}
		err := formatGraphQLError(gqlErr, KindDashboard, "test-dash")

		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "dashboard") {
			t.Errorf("error should mention resource kind: %v", err)
		}
		if !strings.Contains(err.Error(), "test-dash") {
			t.Errorf("error should mention resource name: %v", err)
		}
	})

	t.Run("other errors", func(t *testing.T) {
		otherErr := &testError{msg: "network failure"}
		err := formatGraphQLError(otherErr, KindMonitor, "test-mon")

		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "network failure") {
			t.Errorf("error should contain original message: %v", err)
		}
	})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestExecutionSummary_FailureCount(t *testing.T) {
	tests := []struct {
		name      string
		resources []ResourceResult
		want      int
	}{
		{
			name:      "empty",
			resources: nil,
			want:      0,
		},
		{
			name: "all succeeded",
			resources: []ResourceResult{
				{Kind: KindDashboard, Succeeded: true},
				{Kind: KindMonitor, Succeeded: true},
			},
			want: 0,
		},
		{
			name: "one failure",
			resources: []ResourceResult{
				{Kind: KindDashboard, Succeeded: true},
				{Kind: KindMonitor, Err: &testError{msg: "failed"}},
			},
			want: 1,
		},
		{
			name: "multiple failures",
			resources: []ResourceResult{
				{Kind: KindDashboard, Err: &testError{msg: "failed"}},
				{Kind: KindMonitor, Err: &testError{msg: "failed"}},
				{Kind: KindInsight, Succeeded: true},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ExecutionSummary{Resources: tt.resources}
			if got := s.FailureCount(); got != tt.want {
				t.Errorf("FailureCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPrintExecutionSummary(t *testing.T) {
	t.Run("nil summary", func(t *testing.T) {
		out := &bytes.Buffer{}
		PrintExecutionSummary(out, nil, false)
		if out.Len() != 0 {
			t.Errorf("expected no output for nil summary")
		}
	})

	t.Run("empty resources", func(t *testing.T) {
		out := &bytes.Buffer{}
		PrintExecutionSummary(out, &ExecutionSummary{}, false)
		if out.Len() != 0 {
			t.Errorf("expected no output for empty resources")
		}
	})

	t.Run("dry run message", func(t *testing.T) {
		out := &bytes.Buffer{}
		summary := &ExecutionSummary{
			Resources: []ResourceResult{
				{Kind: KindDashboard, Succeeded: true, Skipped: true},
			},
		}
		PrintExecutionSummary(out, summary, true)
		if !strings.Contains(out.String(), "Dry run complete") {
			t.Errorf("output should mention dry run: %s", out.String())
		}
	})

	t.Run("summary counts", func(t *testing.T) {
		out := &bytes.Buffer{}
		summary := &ExecutionSummary{
			Resources: []ResourceResult{
				{Kind: KindDashboard, Succeeded: true},
				{Kind: KindDashboard, Succeeded: true},
				{Kind: KindMonitor, Err: &testError{msg: "failed"}},
				{Kind: KindInsight, Succeeded: true},
			},
		}
		PrintExecutionSummary(out, summary, false)
		output := out.String()

		if !strings.Contains(output, "dashboard: 2 total, 2 succeeded, 0 failed") {
			t.Errorf("expected dashboard counts in output: %s", output)
		}
		if !strings.Contains(output, "monitor: 1 total, 0 succeeded, 1 failed") {
			t.Errorf("expected monitor counts in output: %s", output)
		}
	})
}

func TestExecutor_BuildVariables_NamespaceID(t *testing.T) {
	t.Run("dry run uses placeholder", func(t *testing.T) {
		client := &mockClient{currentUserID: "user-123"}
		executor := NewExecutor(ExecutorOpts{
			Client: client,
			Out:    &bytes.Buffer{},
			DryRun: true,
		})

		query := "mutation($namespaceId: ID!) { test }"
		vars, err := executor.buildVariables(context.Background(), query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if vars["namespaceId"] != "<dry-run-namespace-id>" {
			t.Errorf("expected placeholder namespace ID, got %v", vars["namespaceId"])
		}

		if client.callCount != 0 {
			t.Errorf("should not call API in dry run mode")
		}
	})

	t.Run("live mode resolves user ID", func(t *testing.T) {
		client := &userMockClient{userID: "user-456"}
		executor := NewExecutor(ExecutorOpts{
			Client: client,
			Out:    &bytes.Buffer{},
			DryRun: false,
		})

		query := "mutation($namespaceId: ID!) { test }"
		vars, err := executor.buildVariables(context.Background(), query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if vars["namespaceId"] != "user-456" {
			t.Errorf("expected user-456, got %v", vars["namespaceId"])
		}
	})

	t.Run("provided namespaceId not overwritten", func(t *testing.T) {
		client := &mockClient{currentUserID: "user-123"}
		executor := NewExecutor(ExecutorOpts{
			Client: client,
			Out:    &bytes.Buffer{},
			Vars:   map[string]string{"namespaceId": "custom-ns"},
		})

		query := "mutation($namespaceId: ID!) { test }"
		vars, err := executor.buildVariables(context.Background(), query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if vars["namespaceId"] != "custom-ns" {
			t.Errorf("expected custom-ns, got %v", vars["namespaceId"])
		}
	})
}

func TestExecutor_InsightDashboardLinking(t *testing.T) {
	dir, _ := setupTestBlueprint(t)
	out := &bytes.Buffer{}

	bp := &Blueprint{
		Version: 1,
		Name:    "test",
		Dashboards: []DashboardRef{
			{Name: "test-dashboard"},
		},
		Insights: []InsightRef{
			{Name: "test-insight", Dashboards: []string{"test-dashboard"}},
		},
	}

	client := &trackingMockClient{
		mockClient: mockClient{
			currentUserID: "user-123",
			responses: map[string]mockResponse{
				"createInsightsDashboard": {
					response: map[string]any{
						"createInsightsDashboard": map[string]any{
							"dashboard": map[string]any{"id": "dash-123"},
						},
					},
					ok: true,
				},
				"createLineChartSearchInsight": {
					response: map[string]any{
						"createLineChartSearchInsight": map[string]any{
							"view": map[string]any{"id": "insight-456"},
						},
					},
					ok: true,
				},
				"addInsightViewToDashboard": {
					response: map[string]any{},
					ok:       true,
				},
			},
		},
	}

	executor := NewExecutor(ExecutorOpts{
		Client: client,
		Out:    out,
	})

	_, err := executor.Execute(context.Background(), bp, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !client.linkCalled {
		t.Error("expected addInsightViewToDashboard to be called")
	}

	if !strings.Contains(out.String(), "Linking insights to dashboards") {
		t.Errorf("output should mention linking: %s", out.String())
	}
}

type trackingMockClient struct {
	mockClient
	linkCalled bool
}

type userMockClient struct {
	userID string
}

func (c *userMockClient) NewQuery(query string) api.Request {
	return c.NewRequest(query, nil)
}

func (c *userMockClient) NewRequest(query string, vars map[string]any) api.Request {
	if strings.Contains(query, "currentUser") {
		return &userMockRequest{userID: c.userID}
	}
	return &mockRequest{ok: true}
}

func (c *userMockClient) NewHTTPRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	return nil, nil
}

func (c *userMockClient) Do(req *http.Request) (*http.Response, error) {
	return nil, nil
}

type userMockRequest struct {
	userID string
}

func (r *userMockRequest) Do(ctx context.Context, result any) (bool, error) {
	if ptr, ok := result.(*struct {
		CurrentUser struct {
			ID string `json:"id"`
		} `json:"currentUser"`
	}); ok {
		ptr.CurrentUser.ID = r.userID
	}
	return true, nil
}

func (r *userMockRequest) DoRaw(ctx context.Context, result any) (bool, error) {
	return r.Do(ctx, result)
}

func (c *trackingMockClient) NewRequest(query string, vars map[string]any) api.Request {
	if strings.Contains(query, "addInsightViewToDashboard") {
		c.linkCalled = true
	}
	return c.mockClient.NewRequest(query, vars)
}

func (c *trackingMockClient) NewQuery(query string) api.Request {
	return c.NewRequest(query, nil)
}
