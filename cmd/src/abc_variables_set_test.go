package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	mockapi "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// If we were to do a json marshalling roundtrip, it may break large integer literals.
// This test is here to demonstrate that the compaction approach is working well.
func TestMarshalABCVariableValuePreservesLargeIntegerLiteral(t *testing.T) {
	t.Parallel()

	value, remove, err := marshalABCVariableValue("9007199254740993")
	if err != nil {
		t.Fatalf("marshalABCVariableValue returned error: %v", err)
	}
	if remove {
		t.Fatal("marshalABCVariableValue unexpectedly marked value for removal")
	}
	if value != "9007199254740993" {
		t.Fatalf("marshalABCVariableValue = %q, want %q", value, "9007199254740993")
	}
}

func TestParseABCVariables(t *testing.T) {
	t.Parallel()

	variables, err := parseABCVariables(
		[]string{"prompt=tighten the review criteria", `title="test"`},
		[]string{"checkpoints=[1,2,3]"},
	)
	if err != nil {
		t.Fatalf("parseABCVariables returned error: %v", err)
	}

	if diff := cmp.Diff(variables, map[string]string{
		"prompt":      "\"tighten the review criteria\"",
		"title":       "\"test\"",
		"checkpoints": "[1,2,3]",
	}); diff != "" {
		t.Errorf("err: %v", diff)
	}
}

func TestRunABCVariablesSet(t *testing.T) {
	t.Parallel()

	client := new(mockapi.Client)
	request := &mockapi.Request{Response: `{"data":{"updateAgenticWorkflowInstanceVariables":{"id":"workflow"}}}`}
	output := &bytes.Buffer{}
	variables := map[string]string{
		"prompt":      `"tighten the review criteria"`,
		"checkpoints": "[1,2,3]",
	}

	client.On("NewRequest", updateABCWorkflowInstanceVariablesMutation, map[string]any{
		"instanceID": "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==",
		"variables": []map[string]string{
			{"key": "checkpoints", "value": "[1,2,3]"},
			{"key": "prompt", "value": `"tighten the review criteria"`},
		},
	}).Return(request).Once()
	request.On("Do", context.Background(), mock.Anything).Return(true, nil).Once()

	err := runABCVariablesSet(context.Background(), client, "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==", variables, output)
	require.NoError(t, err)
	require.Equal(t, "Updated 2 variables on workflow instance \"QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==\".\n", output.String())

	client.AssertExpectations(t)
	request.AssertExpectations(t)
}

func TestRunABCVariablesSetSuppressesSuccessMessageWhenRequestDoesNotExecute(t *testing.T) {
	t.Parallel()

	client := new(mockapi.Client)
	request := &mockapi.Request{}
	output := &bytes.Buffer{}

	client.On("NewRequest", updateABCWorkflowInstanceVariablesMutation, map[string]any{
		"instanceID": "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==",
		"variables":  []map[string]string{{"key": "prompt", "value": `"tighten the review criteria"`}},
	}).Return(request).Once()
	request.On("Do", context.Background(), mock.Anything).Return(false, nil).Once()

	err := runABCVariablesSet(context.Background(), client, "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==", map[string]string{"prompt": `"tighten the review criteria"`}, output)
	require.NoError(t, err)
	require.Empty(t, output.String())

	client.AssertExpectations(t)
	request.AssertExpectations(t)
}
