# Bug Reproducer: Missing Edits in Large Batch Changes

## Overview

This document describes how to reproduce a bug where the CLI sometimes drops edits when processing very large batch changes. Our testing shows that the bug is more common than initially thought and can be reproduced consistently with our test batch spec.

## Symptoms

The bug manifests when the CLI drops edits during processing, resulting in empty diffs. When this happens, you'll see an error like this:

```
❌ Error:
   {
     "message": "2 errors occurred:\n\t* Must validate one and only one schema (oneOf)\n\t* commits.0: diff is required",
     "path": [
       "createChangesetSpec"
     ]
   }
```

## Prerequisites

- Go development environment
- Source code for the CLI with your modifications to `batch_common.go` and `run_steps.go`
- The provided `batch-spec.yaml` file (contains thousands of directory-branch mappings)
- Sourcegraph API credentials

## Reproduction Steps

### Environment Setup

1. Ensure you have proper authentication by setting these environment variables:
   ```bash
   export SRC_ENDPOINT=https://sourcegraph.sourcegraph.com
   export SRC_ACCESS_TOKEN=your_access_token_here
   ```

### Manual Method

1. Run the following command:
   ```
   go run ./cmd/src batch preview -f batch-spec.yaml
   ```

2. With the provided batch spec, the error should appear consistently or within a few attempts.

### Automated Method

1. Use the provided reproducer script:
   ```
   chmod +x reproducer.sh
   ./reproducer.sh
   ```

2. The script will run the command in a loop until it either reproduces the bug or reaches the maximum number of attempts.

3. When the bug occurs, the script will print the error message and exit.

## Analysis

The bug exhibits the following characteristics:

1. It's consistently reproducible with large batch specs (over 4000 changesets).

2. The error message indicates that the system is generating empty diffs, which violates the schema requirement.

3. The root cause appears to be in the processing of large numbers of edits, where some edits are dropped during the creating of changeset specs.

4. Your modifications to `batch_common.go` and `run_steps.go` are successfully catching this bug by ensuring diffs aren't empty.

## Potential Causes

Based on the observed behavior, potential causes include:

1. Memory management issues when handling thousands of changesets
2. Race conditions in parallel processing of batch changes
3. Buffer overflows or edge cases in the diff generation code
4. Unexpected interactions between caching and changeset creation

The consistent reproduction with our test case should make it easier to diagnose and fix the underlying issue.