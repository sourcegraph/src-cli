package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

const updateABCWorkflowInstanceVariablesMutation = `mutation UpdateAgenticWorkflowInstanceVariables(
	$instanceID: ID!,
	$variables: [AgenticWorkflowInstanceVariableInput!]!,
) {
	updateAgenticWorkflowInstanceVariables(instanceID: $instanceID, variables: $variables) {
		id
	}
}`

var abcVariablesSetCommand = clicompat.Wrap(&cli.Command{
	Name:      "set",
	UsageText: "src abc variables set [options] <workflow-instance-id> [<name>=<value> ...]",
	Usage:     "Set variables on a workflow instance",
	Description: `
Set workflow instance variables

Examples:

  Set a string variable on a workflow instance:

    	$ src abc variables set QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== prompt="tighten the review criteria"

  Set multiple variables in one request:

    	$ src abc variables set QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== --var prompt="tighten the review criteria" --var checkpoints='[1,2,3]'

  Set a structured JSON value:

	    $ src abc variables set QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== checkpoints='[1,2,3]'

NOTE: Values are interpreted as JSON literals when valid. Otherwise they are sent as plain strings.
`,
	Flags: clicompat.WithAPIFlags(
		&cli.StringSliceFlag{
			Name:  "var",
			Usage: "Variable assignment in <name>=<value> form. Repeat to set multiple variables.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cmderrors.Usage("must provide a workflow instance ID")
		}

		instanceID := cmd.Args().First()
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)
		return runABCVariablesSet(ctx, client, instanceID, cmd.Args().Tail(), abcVariableArgs(cmd.StringSlice("var")), cmd.Writer, cmd.Bool("get-curl"))
	},
})

type abcVariableArgs []string

func (a *abcVariableArgs) String() string {
	return strings.Join(*a, ",")
}

func (a *abcVariableArgs) Set(value string) error {
	*a = append(*a, value)
	return nil
}

type abcVariable struct {
	Key   string
	Value string
}

func parseABCVariables(positional []string, flagged abcVariableArgs) ([]abcVariable, error) {
	rawVariables := append([]string{}, positional...)
	rawVariables = append(rawVariables, flagged...)
	if len(rawVariables) == 0 {
		return nil, cmderrors.Usage("must provide at least one variable assignment")
	}

	variables := make([]abcVariable, 0, len(rawVariables))
	for _, rawVariable := range rawVariables {
		variable, err := parseABCVariable(rawVariable)
		if err != nil {
			return nil, err
		}
		variables = append(variables, variable)
	}

	return variables, nil
}

func parseABCVariable(raw string) (abcVariable, error) {
	name, rawValue, ok := strings.Cut(raw, "=")
	if !ok || name == "" {
		return abcVariable{}, cmderrors.Usagef("invalid variable assignment %q: must be in <name>=<value> form", raw)
	}

	value, remove, err := marshalABCVariableValue(rawValue)
	if err != nil {
		return abcVariable{}, err
	}
	if remove {
		return abcVariable{}, cmderrors.Usagef("invalid variable assignment %q: use 'src abc variables delete <workflow-instance-id> %s' to remove a variable", raw, name)
	}

	return abcVariable{Key: name, Value: value}, nil
}

func runABCVariablesSet(ctx context.Context, client api.Client, instanceID string, positional []string, flagged abcVariableArgs, output io.Writer, getCurl bool) error {
	variables, err := parseABCVariables(positional, flagged)
	if err != nil {
		return err
	}

	graphqlVariables := make([]map[string]string, 0, len(variables))
	for _, variable := range variables {
		graphqlVariables = append(graphqlVariables, map[string]string{
			"key":   variable.Key,
			"value": variable.Value,
		})
	}

	if err := updateABCWorkflowInstanceVariables(ctx, client, instanceID, graphqlVariables); err != nil {
		return err
	}

	if getCurl {
		return nil
	}

	if len(variables) == 1 {
		_, err = fmt.Fprintf(output, "Set variable %q on workflow instance %q.\n", variables[0].Key, instanceID)
		return err
	}

	_, err = fmt.Fprintf(output, "Updated %d variables on workflow instance %q.\n", len(variables), instanceID)
	return err
}

func updateABCWorkflowInstanceVariables(ctx context.Context, client api.Client, instanceID string, variables []map[string]string) error {
	var result struct {
		UpdateAgenticWorkflowInstanceVariables struct {
			ID string `json:"id"`
		} `json:"updateAgenticWorkflowInstanceVariables"`
	}
	if ok, err := client.NewRequest(updateABCWorkflowInstanceVariablesMutation, map[string]any{
		"instanceID": instanceID,
		"variables":  variables,
	}).Do(ctx, &result); err != nil || !ok {
		return err
	}

	return nil
}

func marshalABCVariableValue(raw string) (value string, remove bool, err error) {
	// Try to compact valid JSON literals first so numbers, arrays, and objects are sent unchanged.
	// A bare null is detected separately so the CLI can require the explicit delete command.
	// If compacting doesn't work for the given value, fall back to string encoding.
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err == nil {
		value := compact.String()
		return value, value == "null", nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", false, err
	}

	return string(encoded), false, nil
}
