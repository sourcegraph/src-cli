package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const McpURLPath = ".api/mcp/v1"

func FetchToolDefinitions(ctx context.Context, client api.Client) (map[string]*ToolDef, error) {
	resp, err := doJSONRPC(ctx, client, "tools/list", nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list tools from mcp endpoint")
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read mcp response data")
	}
	return loadToolDefinitions(data)
}

func DoToolCall(ctx context.Context, client api.Client, tool string, vars map[string]any) (*http.Response, error) {
	params := struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}{
		Name:      tool,
		Arguments: vars,
	}

	return doJSONRPC(ctx, client, "tools/call", params)
}

func doJSONRPC(ctx context.Context, client api.Client, method string, params any) (*http.Response, error) {
	jsonRPC := struct {
		Version string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{
		Version: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}
	buf := bytes.NewBuffer(nil)
	data, err := json.Marshal(jsonRPC)
	if err != nil {
		return nil, err
	}
	buf.Write(data)

	req, err := client.NewHTTPRequest(ctx, http.MethodPost, McpURLPath, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "*/*")

	return client.Do(req)
}

func DecodeToolResponse(resp *http.Response) (map[string]json.RawMessage, error) {
	data, err := readSSEResponseData(resp)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return map[string]json.RawMessage{}, nil
	}

	jsonRPCResp := struct {
		Version string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Content           []json.RawMessage          `json:"content"`
			StructuredContent map[string]json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}{}
	if err := json.Unmarshal(data, &jsonRPCResp); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal MCP JSON-RPC response")
	}

	return jsonRPCResp.Result.StructuredContent, nil
}
func readSSEResponseData(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// The response is an SSE reponse
	// event:
	// data:
	lines := bytes.SplitSeq(data, []byte("\n"))
	for line := range lines {
		if jsonData, ok := bytes.CutPrefix(line, []byte("data: ")); ok {
			return jsonData, nil
		}
	}
	return nil, errors.New("no data found in SSE response")

}
