package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

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

    	$ src abc variables set QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== prompt "tighten the review criteria"

  Remove a variable by setting it to null:

    	$ src abc variables set QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== approval null

  Set a structured JSON value:

    	$ src abc variables set QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== checkpoints '[1,2,3]'

Values are interpreted as JSON literals when valid. Otherwise they are sent as plain strings.
	`

	flagSet := flag.NewFlagSet("set", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src abc variables %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	apiFlags := api.NewFlags(flagSet)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if flagSet.NArg() != 3 {
			return cmderrors.Usage("must provide an instance ID, variable name, and variable value")
		}

		instanceID := flagSet.Arg(0)
		key := flagSet.Arg(1)
		value, remove, err := marshalABCVariableValue(flagSet.Arg(2))
		if err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())
		var result struct {
			UpdateAgenticWorkflowInstanceVariables struct {
				ID string `json:"id"`
			} `json:"updateAgenticWorkflowInstanceVariables"`
		}
		if ok, err := client.NewRequest(updateABCWorkflowInstanceVariablesMutation, map[string]any{
			"instanceID": instanceID,
			"variables": []map[string]string{{
				"key":   key,
				"value": value,
			}},
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		if remove {
			fmt.Printf("Removed variable %q from workflow instance %q.\n", key, instanceID)
			return nil
		}

		fmt.Printf("Set variable %q on workflow instance %q.\n", key, instanceID)
		return nil
	}

	abcVariablesCommands = append(abcVariablesCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func marshalABCVariableValue(raw string) (value string, remove bool, err error) {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		parsed = raw
	}

	encoded, err := json.Marshal(parsed)
	if err != nil {
		return "", false, err
	}

	return string(encoded), parsed == nil, nil
}
