# Fix Idea 2: Race Conditions in Parallel Processing

## Problem
The batch processing system runs multiple operations in parallel, which could lead to race conditions when accessing shared resources or when handling diffs. These race conditions might cause edits to be dropped silently.

## Proposed Solution
Implement stricter concurrency controls and better synchronization:

1. Review all concurrent operations during batch processing, particularly in `RunSteps` and related functions
2. Add mutex locks around critical sections that modify shared data structures
3. Consider using channels for safer communication between concurrent operations
4. Add transaction-like semantics to ensure all edits within a changeset are processed atomically
5. Implement retry mechanisms for concurrent operations that might fail due to race conditions

## Implementation Areas
- `internal/batches/executor/run_steps.go`: Add synchronization around diff generation and validation
- `cmd/src/batch_common.go`: Review concurrent operations in changeset spec creation
- Consider adding a concurrency limit based on the size of batch specs

## Expected Outcome
Eliminate race conditions that could lead to dropped edits, ensuring that all changes are consistently applied even with high levels of parallelism.