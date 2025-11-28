package main

import (
	"flag"
	"fmt"
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
	fmt.Printf("handling tool %q args: %+v", tool.Name, args)
	return nil
}
