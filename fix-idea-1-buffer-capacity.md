# Fix Idea 1: Buffer Capacity Issues

## Problem
When processing large batch changes with thousands of directory-branch mappings, the system may hit buffer limits in the diff generation process, resulting in silently dropped edits and empty diffs.

## Proposed Solution
Increase buffer sizes and add explicit buffer overflow checks in critical code paths that handle diff generation:

1. Review all buffer allocations in the diff generation process and increase capacity for large batch operations
2. Add explicit bounds checking when writing diffs to buffers
3. Implement fallback mechanisms to chunk or paginate extremely large diffs
4. Add more robust error handling for buffer-related failures

## Implementation Areas
- `internal/batches/diff` package: Review any buffer limits 
- `internal/batches/executor/run_steps.go`: Add additional validation of diff content before processing
- Git related functions: Ensure they can handle large diffs without truncation

## Expected Outcome
The system should either properly handle large diffs without dropping edits or fail with a clear error message about size limitations rather than silently dropping content.