package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	mockapi "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRunABCVariablesDelete(t *testing.T) {
	t.Parallel()

	client := new(mockapi.Client)
	request := &mockapi.Request{Response: `{"data":{"updateAgenticWorkflowInstanceVariables":{"id":"workflow"}}}`}
	output := &bytes.Buffer{}
	variableNames := []string{"approval", "checkpoints", "prompt"}

	client.On("NewRequest", updateABCWorkflowInstanceVariablesMutation, map[string]any{
		"instanceID": "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==",
		"variables": []map[string]string{
			{"key": "approval", "value": "null"},
			{"key": "checkpoints", "value": "null"},
			{"key": "prompt", "value": "null"},
		},
	}).Return(request).Once()
	request.On("Do", context.Background(), mock.Anything).Return(true, nil).Once()

	err := runABCVariablesDelete(context.Background(), client, "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==", variableNames, output, false)
	require.NoError(t, err)
	require.Equal(t, "Removed variables [\"approval\" \"checkpoints\" \"prompt\"] from workflow instance \"QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==\".\n", output.String())

	client.AssertExpectations(t)
	request.AssertExpectations(t)
}

func TestRunABCVariablesDeleteRejectsEmptyVariableName(t *testing.T) {
	t.Parallel()

	err := runABCVariablesDelete(context.Background(), nil, "QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ==", []string{"approval", ""}, io.Discard, false)
	require.ErrorContains(t, err, "variable names must not be empty")
}
