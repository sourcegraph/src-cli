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

const (
	MCPURLPath       = ".api/mcp"
	MCPURLPathLegacy = ".api/mcp/v1"
)

func resolveMCPEndpoint(ctx context.Context, client api.Client) (string, error) {
	for _, endpoint := range []string{MCPURLPath, MCPURLPathLegacy} {
		_, err := doJSONRPC(ctx, client, endpoint, "tools/list", nil)
		if err == nil {
			return endpoint, nil
		}
	}
	return "", errors.Newf("MCP endpoint not available: tried %s and %s", MCPURLPath, MCPURLPathLegacy)
}

func fetchToolDefinitions(ctx context.Context, client api.Client, endpoint string) (map[string]*ToolDef, error) {
	resp, err := doJSONRPC(ctx, client, endpoint, "tools/list", nil)
	if err != nil {
		return nil, errors.Wrapf(err, "JSON-RPC tools/list request failed to %q", MCPURLPath)
	}
	defer resp.Body.Close()

	data, err := readSSEResponseData(resp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read tools/list SSE response")
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

	resp, err := doJSONRPC(ctx, client, endpoint, "tools/call", params)
	if err != nil {
		return nil, errors.Wrapf(err, "JSON-RPC tools/call request failed to %q", MCPURLPath)
	}
	return resp, err
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
			IsError           bool                       `json:"isError"`
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
		return nil, errors.Newf("MCP JSON-RPC error: %d %s", jsonRPCResp.Error.Code, jsonRPCResp.Error.Message)
	}

	if jsonRPCResp.Result.IsError {
		content := jsonRPCResp.Result.Content[0]
		if len(content) > 0 {
			var textContent struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(content, &textContent); err == nil && textContent.Text != "" {
				return nil, errors.Newf("MCP tool error: %s", textContent.Text)
			}
			return nil, errors.Newf("MCP tool error: %s", string(content))
		}
		return nil, errors.New("MCP tool returned an error")
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
