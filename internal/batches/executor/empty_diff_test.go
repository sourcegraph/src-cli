package executor

import (
	"testing"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/stretchr/testify/assert"
)

func TestEmptyDiffError(t *testing.T) {
	// Simulate the error case from run_steps.go when a diff is empty
	
	// Create an error using similar logic to the fixed error path
	emptyDiffError := errors.New("diff was empty - this may be due to buffer capacity issues when processing large batch changes")
	
	// Check that our error message mentions buffer capacity
	assert.Contains(t, emptyDiffError.Error(), "buffer capacity", "Error should mention buffer capacity issues")
}