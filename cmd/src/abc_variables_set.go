package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
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
		abcVariables, err := parseABCVariables(cmd.Args().Tail(), cmd.StringSlice("var"))
		if err != nil {
			return err
		}
		return runABCVariablesSet(ctx, client, instanceID, abcVariables, cmd.Writer)
	},
})

func parseABCVariables(positional []string, flagged []string) (map[string]string, error) {
	rawVariables := append(positional, flagged...)
	if len(rawVariables) == 0 {
		return nil, cmderrors.Usage("must provide at least one variable assignment")
	}

	variables := make(map[string]string, len(rawVariables))
	for _, v := range rawVariables {
		name, rawValue, ok := strings.Cut(v, "=")
		if !ok || name == "" {
			return nil, cmderrors.Usagef("invalid variable assignment %q: must be in <name>=<value> form", v)
		}

		value, remove, err := marshalABCVariableValue(rawValue)
		if err != nil {
			return nil, err
		}
		if remove {
			return nil, cmderrors.Usagef("invalid variable assignment %q: use 'src abc variables delete <workflow-instance-id> %s' to remove a variable", rawValue, name)
		}

		variables[name] = value
	}

	return variables, nil
}

func runABCVariablesSet(ctx context.Context, client api.Client, instanceID string, variables map[string]string, output io.Writer) error {
	graphqlVariables := make([]map[string]string, 0, len(variables))
	keys := make([]string, 0, len(variables))
	for k := range variables {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		graphqlVariables = append(graphqlVariables, map[string]string{
			"key":   k,
			"value": variables[k],
		})
	}

	ok, err := updateABCWorkflowInstanceVariables(ctx, client, instanceID, graphqlVariables)
	if err != nil || !ok {
		return err
	}

	_, err = fmt.Fprintf(output, "Updated %d variables on workflow instance %q.\n", len(variables), instanceID)
	return err
}

func updateABCWorkflowInstanceVariables(ctx context.Context, client api.Client, instanceID string, variables []map[string]string) (bool, error) {
	var result struct {
		UpdateAgenticWorkflowInstanceVariables struct {
			ID string `json:"id"`
		} `json:"updateAgenticWorkflowInstanceVariables"`
	}
	if ok, err := client.NewRequest(updateABCWorkflowInstanceVariablesMutation, map[string]any{
		"instanceID": instanceID,
		"variables":  variables,
	}).Do(ctx, &result); err != nil || !ok {
		return ok, err
	}

	return true, nil
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
