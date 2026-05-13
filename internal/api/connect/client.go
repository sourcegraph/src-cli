// Package connect provides a minimal JSON client for Sourcegraph's public
// Connect RPC APIs (https://connectrpc.com/) under /api.
//
// It is a thin layer over internal/api so that authentication, proxying, TLS,
// tracing, additional headers, and diagnostic flags (-get-curl,
// -dump-requests, -trace) all share behavior with GraphQL requests.
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
)

const protocolVersion = "1"

// Client calls Connect RPC unary endpoints using JSON encoding.
type Client interface {
	// NewCall creates a Call for procedure with the given request payload.
	//
	// procedure should be the fully-qualified Connect procedure path, e.g.
	// "/deepsearch.v1.Service/GetConversation".
	NewCall(procedure string, request any) Call
}

// Call represents a unary Connect RPC.
type Call interface {
	// Do sends the request and decodes the JSON response into response.
	//
	// ok is false when the request was intentionally not sent, such as when
	// -get-curl is set. If response is nil, the body is discarded.
	Do(ctx context.Context, response any) (ok bool, err error)
}

// NewClient creates a Connect JSON client on top of an api.Client. Diagnostic
// flags and the output writer are taken from the underlying api.Client so
// behavior matches GraphQL requests.
func NewClient(apiClient api.Client) Client {
	return &client{apiClient: apiClient}
}

type client struct {
	apiClient api.Client
}

func (c *client) NewCall(procedure string, request any) Call {
	return &call{
		client:    c,
		procedure: procedure,
		request:   request,
	}
}

type call struct {
	client    *client
	procedure string
	request   any
}

func (c *call) Do(ctx context.Context, response any) (bool, error) {
	body, err := json.Marshal(c.request)
	if err != nil {
		return false, err
	}

	req, err := c.client.apiClient.NewHTTPRequest(ctx, http.MethodPost, procedurePath(c.procedure), bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Connect-Protocol-Version", protocolVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	flags := c.client.apiClient.Flags()
	out := c.client.apiClient.Out()
	if out == nil {
		out = io.Discard
	}

	if flags != nil && flags.GetCurl() {
		_, err := fmt.Fprintln(out, api.CurlCommand(req, body))
		return false, err
	}

	if flags != nil && flags.Dump() {
		fmt.Fprintf(out, "<-- connect request %s:\n%s\n\n", c.procedure, prettyJSON(body))
	}

	resp, err := c.client.apiClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if flags != nil && flags.Trace() {
		if _, err := fmt.Fprintf(out, "x-trace: %s\n", resp.Header.Get("x-trace")); err != nil {
			return false, err
		}
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if flags != nil && flags.Dump() {
		fmt.Fprintf(out, "--> %s\n\n", prettyJSON(responseBody))
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, parseError(resp.Status, responseBody)
	}

	if response == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return true, nil
	}
	if err := json.Unmarshal(responseBody, response); err != nil {
		return false, err
	}
	return true, nil
}

// procedurePath joins the Connect procedure name onto the /api route. The
// leading slash is trimmed so that api.Client.NewHTTPRequest can build the
// final URL by appending to EndpointURL.
func procedurePath(procedure string) string {
	return "api/" + strings.TrimPrefix(procedure, "/")
}

func prettyJSON(data []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", "  "); err == nil {
		return out.String()
	}
	return string(data)
}
