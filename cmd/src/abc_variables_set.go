package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const updateABCWorkflowInstanceVariablesMutation = `mutation UpdateAgenticWorkflowInstanceVariables(
	$instanceID: ID!,
	$variables: [AgenticWorkflowInstanceVariableInput!]!,
) {
	updateAgenticWorkflowInstanceVariables(instanceID: $instanceID, variables: $variables) {
		id
	}
}`

func init() {
	usage := `
Examples:

  Set a string variable on a workflow instance:

    	$ src abc QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== variables set prompt="tighten the review criteria"

  Set multiple variables in one request:

    	$ src abc QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== variables set --var prompt="tighten the review criteria" --var checkpoints='[1,2,3]'

  Set a structured JSON value:

	    $ src abc QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== variables set checkpoints='[1,2,3]'

Values are interpreted as JSON literals when valid. Otherwise they are sent as plain strings.
	`

	flagSet := flag.NewFlagSet("set", flag.ExitOnError)
	var variableArgs abcVariableArgs
	flagSet.Var(&variableArgs, "var", "Variable assignment in <name>=<value> form. Repeat to set multiple variables.")
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src abc <workflow-instance-id> variables %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	apiFlags := api.NewFlags(flagSet)

	handler := func(args []string) error {
		if len(args) == 0 {
			return cmderrors.Usage("must provide a workflow instance ID")
		}

		instanceID := args[0]
		variableArgs = nil
		if err := flagSet.Parse(args[1:]); err != nil {
			return err
		}

		variables, err := parseABCVariables(flagSet.Args(), variableArgs)
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

		client := cfg.apiClient(apiFlags, flagSet.Output())
		if err := updateABCWorkflowInstanceVariables(context.Background(), client, instanceID, graphqlVariables); err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		if len(variables) == 1 {
			fmt.Printf("Set variable %q on workflow instance %q.\n", variables[0].Key, instanceID)
			return nil
		}

		fmt.Printf("Updated %d variables on workflow instance %q.\n", len(variables), instanceID)
		return nil
	}

	abcVariablesCommands = append(abcVariablesCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

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
		return abcVariable{}, cmderrors.Usagef("invalid variable assignment %q: use 'src abc <workflow-instance-id> variables delete %s' to remove a variable", raw, name)
	}

	return abcVariable{Key: name, Value: value}, nil
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
