package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/mcp"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const McpPath = ".api/mcp/v1"

func init() {
	flagSet := flag.NewFlagSet("mcp", flag.ExitOnError)
	commands = append(commands, &command{
		flagSet: flagSet,
		handler: mcpMain,
	})
}
func mcpMain(args []string) error {
	fmt.Println("NOTE: This command is still experimental")
	tools, err := mcp.LoadDefaultToolDefinitions()
	if err != nil {
		return err
	}

	subcmd := args[0]
	if subcmd == "list-tools" {
		fmt.Println("The following tools are available:")
		for name := range tools {
			fmt.Printf(" • %s\n", name)
		}
		fmt.Println("\nUSAGE:")
		fmt.Printf(" • Invoke a tool\n")
		fmt.Printf("     src mcp <tool-name> <flags>\n")
		fmt.Printf("\n • View the Input / Output Schema of a tool\n")
		fmt.Printf("     src mcp <tool-name> schema\n")
		fmt.Printf("\n • List the available flags of a tool\n")
		fmt.Printf("     src mcp <tool-name> -h\n")
		fmt.Printf("\n • View the Input / Output Schema of a tool\n")
		fmt.Printf("     src mcp <tool-name> schema\n")
		return nil
	}

	tool, ok := tools[subcmd]
	if !ok {
		return fmt.Errorf("tool definition for %q not found - run src mcp list-tools to see a list of available tools", subcmd)
	}

	flagArgs := args[1:] // skip subcommand name
	if len(args) > 1 && args[1] == "schema" {
		return printSchemas(tool)
	}

	flags, vars, err := mcp.BuildToolFlagSet(tool)
	if err != nil {
		return err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	sanitizeFlagValues(vars)

	if err := validateToolArgs(tool.InputSchema, args, vars); err != nil {
		return err
	}

	apiClient := cfg.apiClient(nil, flags.Output())
	return handleMcpTool(context.Background(), apiClient, tool, vars)
}

func printSchemas(tool *mcp.ToolDef) error {
	input, err := json.MarshalIndent(tool.InputSchema, "", " ")
	if err != nil {
		return err
	}
	output, err := json.MarshalIndent(tool.OutputSchema, "", " ")
	if err != nil {
		return err
	}

	fmt.Printf("Input:\n%v\nOutput:\n%v\n", string(input), string(output))
	return nil
}

func validateToolArgs(inputSchema mcp.Schema, args []string, vars map[string]any) error {
	for _, reqName := range inputSchema.Required {
		if vars[reqName] == nil {
			return errors.Newf("no value provided for required flag --%s", reqName)
		}
	}

	if len(args) < len(inputSchema.Required) {
		return errors.Newf("not enough arguments provided - the following flags are required:\n%s", strings.Join(inputSchema.Required, "\n"))
	}

	return nil
}

func handleMcpTool(ctx context.Context, client api.Client, tool *mcp.ToolDef, vars map[string]any) error {
	jsonRPC := struct {
		Version string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{
		Version: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}{
			Name:      tool.RawName,
			Arguments: vars,
		},
	}

	buf := bytes.NewBuffer(nil)
	data, err := json.Marshal(jsonRPC)
	if err != nil {
		return err
	}
	buf.Write(data)

	req, err := client.NewHTTPRequest(ctx, http.MethodPost, McpPath, buf)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	jsonData, err := parseSSEResponse(data)
	if err != nil {
		return err
	}

	fmt.Println(string(jsonData))
	return nil
}

func parseSSEResponse(data []byte) ([]byte, error) {
	lines := bytes.SplitSeq(data, []byte("\n"))
	for line := range lines {
		if jsonData, ok := bytes.CutPrefix(line, []byte("data: ")); ok {
			return jsonData, nil
		}
	}
	return nil, errors.New("no data found in SSE response")
}
