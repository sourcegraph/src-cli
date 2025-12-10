package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const MCPURLPath = ".api/mcp/v1"
const MCPDeepSearchURLPath = ".api/mcp/deepsearch"

func fetchToolDefinitions(ctx context.Context, client api.Client, endpoint string) (map[string]*ToolDef, error) {
	resp, err := doJSONRPC(ctx, client, endpoint, "tools/list", nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list tools from mcp endpoint")
	}
	defer resp.Body.Close()

	data, err := readSSEResponseData(resp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read list tools SSE response")
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON-RPC response")
	}
	if rpcResp.Error != nil {
		return nil, errors.Newf("MCP tools/list failed: %d %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return loadToolDefinitions(rpcResp.Result)
}

func doToolCall(ctx context.Context, client api.Client, endpoint string, tool string, vars map[string]any) (*http.Response, error) {
	params := struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}{
		Name:      tool,
		Arguments: vars,
	}

	return doJSONRPC(ctx, client, endpoint, "tools/call", params)
}

func doJSONRPC(ctx context.Context, client api.Client, endpoint string, method string, params any) (*http.Response, error) {
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

	req, err := client.NewHTTPRequest(ctx, http.MethodPost, endpoint, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json, text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, errors.Newf("MCP endpoint %s returned %d: %s",
			endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return resp, nil
}

func decodeToolResponse(resp *http.Response) (map[string]json.RawMessage, error) {
	data, err := readSSEResponseData(resp)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return map[string]json.RawMessage{}, nil
	}

	jsonRPCResp := struct {
		Result struct {
			Content           []json.RawMessage          `json:"content"`
			StructuredContent map[string]json.RawMessage `json:"structuredContent"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}{}
	if err := json.Unmarshal(data, &jsonRPCResp); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal MCP JSON-RPC response")
	}
	if jsonRPCResp.Error != nil {
		return nil, errors.Newf("MCP tools/call failed: %d %s", jsonRPCResp.Error.Code, jsonRPCResp.Error.Message)
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
