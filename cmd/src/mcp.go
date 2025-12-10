package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/mcp"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var mcpFlagSet = flag.NewFlagSet("mcp", flag.ExitOnError)

func init() {
	commands = append(commands, &command{
		flagSet:   mcpFlagSet,
		handler:   mcpMain,
		usageFunc: mcpUsage,
	})
}

func mcpUsage() {
	fmt.Println("The 'mcp' command exposes MCP tools as subcommands for agents to use.")
	fmt.Println("\nUSAGE:")
	fmt.Println("  src mcp list-tools              List available tools")
	fmt.Println("  src mcp <tool-name> schema      View the input/output schema of a tool")
	fmt.Println("  src mcp <tool-name> <flags>     Invoke a tool with the given flags")
	fmt.Println("  src mcp <tool-name> -h          List the available flags of a tool")
}

func mcpMain(args []string) error {
	fmt.Println("NOTE: This command is still experimental")
	apiClient := cfg.apiClient(nil, mcpFlagSet.Output())

	ctx := context.Background()
	registry := mcp.NewToolRegistry()
	if err := registry.LoadTools(ctx, apiClient); err != nil {
		return err
	}

	if len(args) == 0 {
		mcpUsage()
		return nil
	}

	subcmd := args[0]
	if subcmd == "list-tools" {
		fmt.Println("The following tools are available:")
		for name := range registry.All() {
			fmt.Printf("  %s\n", name)
		}
		fmt.Println("\nUSAGE:")
		fmt.Println("  src mcp <tool-name> schema      View the input/output schema of a tool")
		fmt.Println("  src mcp <tool-name> <flags>     Invoke a tool with the given flags")
		fmt.Println("  src mcp <tool-name> -h          List the available flags of a tool")
		return nil
	}
	tool, ok := registry.Get(subcmd)
	if !ok {
		return errors.Newf("tool definition for %q not found - run src mcp list-tools to see a list of available tools", subcmd)
	}

	flagArgs := args[1:] // skip subcommand name
	if len(args) > 1 && args[1] == "schema" {
		return printSchemas(tool)
	}

	flags, vars, err := mcp.BuildArgFlagSet(tool)
	if err != nil {
		return err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	mcp.DerefFlagValues(flags, vars)

	if err := validateToolArgs(tool.InputSchema, args, vars); err != nil {
		return err
	}

	result, err := registry.CallTool(ctx, apiClient, tool.Name, vars)
	if err != nil {
		return err
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
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

func validateToolArgs(inputSchema mcp.SchemaObject, args []string, vars map[string]any) error {
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
