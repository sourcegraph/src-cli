package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches/service"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var streamTestCommand = clicompat.Wrap(&cli.Command{
	Name:        "stream-test",
	Usage:       "prototype Batch Changes executor log streaming",
	UsageText:   "src stream-test [options]",
	Description: "Starts a tiny server-side batch change that emits enough output to exercise executor log streaming, then prints stream chunks as they appear.",
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "sourcegraph-dir",
			Value: filepath.Join("..", "sourcegraph"),
			Usage: "Path to the Sourcegraph checkout used to run sg db env.",
		},
		&cli.BoolFlag{
			Name:  "skip-sg-db-env",
			Usage: "Do not run sg db env before connecting to Sourcegraph.",
		},
		&cli.StringFlag{
			Name:  "repo",
			Value: "github.com/sourcegraph/sourcegraph",
			Usage: "Repository to target with the prototype batch spec.",
		},
		&cli.StringFlag{
			Name:  "namespace",
			Usage: "User or organization namespace for the prototype batch change. Defaults to the current user.",
		},
		&cli.DurationFlag{
			Name:  "poll-interval",
			Value: time.Second,
			Usage: "How often to poll execution logs.",
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Value: 10 * time.Minute,
			Usage: "Maximum time to wait for the prototype execution.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Bool("skip-sg-db-env") {
			if err := applySGDBEnv(ctx, cmd.String("sourcegraph-dir")); err != nil {
				return err
			}
		}

		if cmd.Duration("poll-interval") <= 0 {
			return errors.New("poll interval must be positive")
		}
		if cmd.Duration("timeout") <= 0 {
			return errors.New("timeout must be positive")
		}

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.ErrWriter)
		svc := service.New(&service.Opts{Client: client})

		namespace, err := svc.ResolveNamespace(ctx, cmd.String("namespace"))
		if err != nil {
			return err
		}

		name := fmt.Sprintf("stream-test-%s", time.Now().UTC().Format("20060102-150405"))
		batchChangeID, batchChangeName, err := svc.UpsertBatchChange(ctx, name, namespace.ID)
		if err != nil {
			return err
		}

		rawSpec := streamTestBatchSpec(name, cmd.String("repo"))
		batchSpecID, err := svc.CreateBatchSpecFromRaw(ctx, rawSpec, namespace.ID, true, true, true, batchChangeID)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.Writer, "created batch change %s and batch spec %s\n", batchChangeName, batchSpecID)
		if err := waitForStreamTestWorkspaceResolution(ctx, svc, batchSpecID, cmd.Duration("poll-interval")); err != nil {
			return err
		}

		batchSpecID, err = svc.ExecuteBatchSpec(ctx, batchSpecID, true)
		if err != nil {
			return err
		}

		executionURL := cfg.endpointURL.JoinPath(
			fmt.Sprintf("%s/batch-changes/%s/executions/%s", namespace.URL, batchChangeName, batchSpecID),
		).String()
		fmt.Fprintf(cmd.Writer, "started execution %s\n", executionURL)

		pollCtx, cancel := context.WithTimeout(ctx, cmd.Duration("timeout"))
		defer cancel()

		return streamTestLogs(pollCtx, streamTestLogOptions{
			client:       client,
			httpClient:   streamTestHTTPClient(cmd.Bool("insecure-skip-verify")),
			batchSpecID:  batchSpecID,
			pollInterval: cmd.Duration("poll-interval"),
			out:          cmd.Writer,
		})
	},
})

func applySGDBEnv(ctx context.Context, sourcegraphDir string) error {
	cmd := exec.CommandContext(ctx, "./sg", "db", "env")
	cmd.Dir = sourcegraphDir
	data, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "running `sg db env` in %s", sourcegraphDir)
	}

	parsed, err := parseSGDBEnv(string(data))
	if err != nil {
		return err
	}
	for k, v := range parsed {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}

	if endpoint := parsed["SRC_ENDPOINT"]; endpoint != "" {
		endpointURL, err := parseEndpoint(endpoint)
		if err != nil {
			return err
		}
		cfg.endpointURL = endpointURL
	}
	if token := parsed["SRC_ACCESS_TOKEN"]; token != "" {
		cfg.accessToken = token
	}

	return nil
}

func parseSGDBEnv(output string) (map[string]string, error) {
	env := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.Newf("could not parse `sg db env` line %q", line)
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		env[key] = value
	}
	return env, nil
}

func streamTestBatchSpec(name, repo string) string {
	return fmt.Sprintf(`version: 2
name: %s
description: Prototype executor log streaming batch change.
on:
  - repository: %s
steps:
  - run: |
      i=1
      while [ "$i" -le 20000 ]; do
        printf 'stream-test line %%05d ****************************************************************************************************\n' "$i"
        i=$((i + 1))
        if [ $((i %% 500)) -eq 0 ]; then sleep 2; fi
      done
      date -u '+stream-test %%Y-%%m-%%dT%%H:%%M:%%SZ' > stream-test.txt
    container: alpine:3.19
changesetTemplate:
  title: Stream test
  body: Prototype batch change for executor log streaming.
  branch: stream-test
  commit:
    message: Stream test
  published: false
`, name, repo)
}

func waitForStreamTestWorkspaceResolution(ctx context.Context, svc *service.Service, batchSpecID string, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		res, err := svc.GetBatchSpecWorkspaceResolution(ctx, batchSpecID)
		if err != nil {
			return err
		}
		switch res.State {
		case "FAILED":
			return errors.Newf("workspace resolution failed: %s", res.FailureMessage)
		case "COMPLETED":
			if res.Workspaces.TotalCount == 0 {
				return errors.New("workspace resolution completed with no workspaces; choose a repository that exists on the local instance")
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type streamTestLogOptions struct {
	client       api.Client
	httpClient   *http.Client
	batchSpecID  string
	pollInterval time.Duration
	out          io.Writer
}

func streamTestLogs(ctx context.Context, opts streamTestLogOptions) error {
	ticker := time.NewTicker(opts.pollInterval)
	defer ticker.Stop()

	printedChunksByEntry := map[string]int{}
	var finalEntries []streamTestExecutionLogEntry
	var terminalState string
	var terminalFailureMessage string

	for {
		state, entries, terminal, failureMessage, err := fetchStreamTestExecutionLogs(ctx, opts.client, opts.batchSpecID)
		if err != nil {
			return err
		}
		finalEntries = entries

		for _, entry := range entries {
			if entry.LogStream == nil {
				continue
			}
			printed := printedChunksByEntry[entry.Key]
			for chunkIndex := printed + 1; chunkIndex <= entry.LogStream.ChunkCount; chunkIndex++ {
				fmt.Fprintf(opts.out, "\n===== stream-test chunk start: entry=%s chunk=%d/%d byteOffset=%s status=%s =====\n", entry.Key, chunkIndex, entry.LogStream.ChunkCount, entry.LogStream.ByteOffset, entry.LogStream.Status)
				if err := fetchStreamTestChunk(ctx, opts.httpClient, entry.LogStream.StreamURL, chunkIndex, opts.out); err != nil {
					return err
				}
				printedChunksByEntry[entry.Key] = chunkIndex
			}
		}

		if terminal {
			terminalState = state
			terminalFailureMessage = failureMessage
			fmt.Fprintf(opts.out, "\n===== stream-test execution terminal: state=%s =====\n", state)
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	for _, entry := range finalEntries {
		if entry.Out == "" {
			continue
		}
		byteOffset := "0"
		if entry.LogStream != nil {
			byteOffset = entry.LogStream.ByteOffset
		}
		fmt.Fprintf(opts.out, "\n===== stream-test final out tail start: entry=%s byteOffset=%s =====\n", entry.Key, byteOffset)
		if err := writeStreamTestOutput(opts.out, entry.Out); err != nil {
			return err
		}
	}
	if terminalState == "FAILED" || terminalState == "CANCELED" {
		if terminalFailureMessage != "" {
			return errors.Newf("workspace execution ended in state %s: %s", terminalState, terminalFailureMessage)
		}
		return errors.Newf("workspace execution ended in state %s", terminalState)
	}

	return nil
}

const streamTestExecutionLogsQuery = `
query StreamTestExecutionLogs($batchSpec: ID!) {
  node(id: $batchSpec) {
    ... on BatchSpec {
      workspaceResolution {
        state
        failureMessage
        workspaces(first: 1) {
          nodes {
            id
            state
            ... on VisibleBatchSpecWorkspace {
              failureMessage
              stages {
                srcExec {
                  key
                  exitCode
                  out
                  logStream {
                    chunkCount
                    byteOffset
                    streamURL
                    status
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
`

type streamTestExecutionLogEntry struct {
	Key       string               `json:"key"`
	ExitCode  *int                 `json:"exitCode"`
	Out       string               `json:"out"`
	LogStream *streamTestLogStream `json:"logStream"`
}

type streamTestLogStream struct {
	ChunkCount int    `json:"chunkCount"`
	ByteOffset string `json:"byteOffset"`
	StreamURL  string `json:"streamURL"`
	Status     string `json:"status"`
}

func fetchStreamTestExecutionLogs(ctx context.Context, client api.Client, batchSpecID string) (string, []streamTestExecutionLogEntry, bool, string, error) {
	var result struct {
		Data struct {
			Node struct {
				WorkspaceResolution struct {
					State          string `json:"state"`
					FailureMessage string `json:"failureMessage"`
					Workspaces     struct {
						Nodes []struct {
							ID             string  `json:"id"`
							State          string  `json:"state"`
							FailureMessage *string `json:"failureMessage"`
							Stages         struct {
								SrcExec []streamTestExecutionLogEntry `json:"srcExec"`
							} `json:"stages"`
						} `json:"nodes"`
					} `json:"workspaces"`
				} `json:"workspaceResolution"`
			} `json:"node"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}

	if ok, err := client.NewRequest(streamTestExecutionLogsQuery, map[string]any{"batchSpec": batchSpecID}).DoRaw(ctx, &result); err != nil || !ok {
		return "", nil, false, "", err
	}
	if len(result.Errors) > 0 {
		formatted, _ := json.Marshal(result.Errors)
		return "", nil, false, "", errors.Newf("GraphQL errors querying execution logs: %s", formatted)
	}

	resolution := result.Data.Node.WorkspaceResolution
	if resolution.State == "FAILED" {
		return resolution.State, nil, true, resolution.FailureMessage, errors.Newf("workspace resolution failed: %s", resolution.FailureMessage)
	}
	if len(resolution.Workspaces.Nodes) == 0 {
		return resolution.State, nil, false, "", nil
	}

	workspace := resolution.Workspaces.Nodes[0]
	failureMessage := ""
	if workspace.FailureMessage != nil {
		failureMessage = *workspace.FailureMessage
	}
	return workspace.State, workspace.Stages.SrcExec, isStreamTestTerminalState(workspace.State), failureMessage, nil
}

func isStreamTestTerminalState(state string) bool {
	switch state {
	case "COMPLETED", "FAILED", "CANCELED":
		return true
	default:
		return false
	}
}

func streamTestHTTPClient(insecureSkipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Mirrors the existing API flag for this local prototype.
	}
	return &http.Client{Transport: transport}
}

func fetchStreamTestChunk(ctx context.Context, client *http.Client, streamURL string, chunkIndex int, out io.Writer) error {
	chunkURL, err := resolveStreamTestChunkURL(streamURL, chunkIndex)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chunkURL, nil)
	if err != nil {
		return err
	}
	if jobID := streamTestJobIDFromStreamURL(streamURL); jobID != "" {
		req.Header.Set("X-Sourcegraph-Job-ID", jobID)
	}
	if cfg.accessToken != "" {
		req.Header.Set("Authorization", "token "+cfg.accessToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return errors.Newf("fetching stream chunk %d failed with status %s: %s", chunkIndex, resp.Status, strings.TrimSpace(string(body)))
	}

	displayOut := &streamTestOutputWriter{w: out}
	if _, err := io.Copy(displayOut, resp.Body); err != nil {
		return err
	}
	return displayOut.Flush()
}

func resolveStreamTestChunkURL(streamURL string, chunkIndex int) (string, error) {
	parsed, err := url.Parse(streamURL)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() {
		parsed = cfg.endpointURL.ResolveReference(parsed)
	}
	return parsed.JoinPath(fmt.Sprintf("%d.chunk", chunkIndex)).String(), nil
}

func streamTestJobIDFromStreamURL(streamURL string) string {
	parsed, err := url.Parse(streamURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "streams" {
			return parts[i+2]
		}
	}
	return ""
}

func writeStreamTestOutput(out io.Writer, data string) error {
	displayOut := &streamTestOutputWriter{w: out}
	if _, err := displayOut.Write([]byte(data)); err != nil {
		return err
	}
	return displayOut.Flush()
}

type streamTestOutputWriter struct {
	w                io.Writer
	pendingBackslash bool
}

func (w *streamTestOutputWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if w.pendingBackslash {
			w.pendingBackslash = false
			if b == 'n' {
				if _, err := w.w.Write([]byte{'\n'}); err != nil {
					return 0, err
				}
				continue
			}
			if _, err := w.w.Write([]byte{'\\'}); err != nil {
				return 0, err
			}
		}

		if b == '\\' {
			w.pendingBackslash = true
			continue
		}
		if _, err := w.w.Write([]byte{b}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *streamTestOutputWriter) Flush() error {
	if !w.pendingBackslash {
		return nil
	}
	w.pendingBackslash = false
	_, err := w.w.Write([]byte{'\\'})
	return err
}
