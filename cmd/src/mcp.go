package main

import (
	"flag"
	"fmt"
	"strings"
)

func init() {
	flagSet := flag.NewFlagSet("mcp", flag.ExitOnError)
	commands = append(commands, &command{
		flagSet: flagSet,
		handler: mcpMain,
	})
}
func mcpMain(args []string) error {
	fmt.Println("NOTE: This command is still experimental")
	tools, err := LoadMCPToolDefinitions(mcpToolListJSON)
	if err != nil {
		return err
	}

	subcmd := args[0]
	if subcmd == "list-tools" {
		fmt.Println("Available tools")
		for name := range tools {
			fmt.Printf("- %s\n", name)
		}
		return nil
	}

	tool, ok := tools[subcmd]
	if !ok {
		return fmt.Errorf("tool definition for %q not found - run src mcp list-tools to see a list of available tools", subcmd)
	}
	return handleMcpTool(tool, args[1:])
}

func handleMcpTool(tool *MCPToolDef, args []string) error {
	fs, vars, err := buildArgFlagSet(tool)
	if err != nil {
		return err
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	inputSchema := tool.InputSchema

	for _, reqName := range inputSchema.Required {
		if vars[reqName] == nil {
			return fmt.Errorf("no value provided for required flag --%s", reqName)
		}
	}

	if len(args) < len(inputSchema.Required) {
		return fmt.Errorf("not enough arguments provided - the following flags are required:\n%s", strings.Join(inputSchema.Required, "\n"))
	}

	derefFlagValues(vars)

	fmt.Println("Flags")
	for name, val := range vars {
		fmt.Printf("--%s=%v\n", name, val)
	}

	return nil
}
