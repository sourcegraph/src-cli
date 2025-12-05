package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/mcp"
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
	return handleMcpTool(tool, args[1:])
}

func handleMcpTool(tool *mcp.ToolDef, args []string) error {
	fs, vars, err := mcp.BuildArgFlagSet(tool)
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

	mcp.DerefFlagValues(vars)

	fmt.Println("Flags")
	for name, val := range vars {
		fmt.Printf("--%s=%v\n", name, val)
	}

	return nil
}
