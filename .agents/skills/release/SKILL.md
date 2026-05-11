---
name: release
description: Guides Sourcegraph CLI patch, minor, and major releases. Use when preparing release branches, selecting patch commits, running Trivy checks, creating release tags, or managing local/pushed release artifacts.
---

# Release

## Infer the release plan

Gather only the information that is actually missing. Do not ask questions whose answers can be derived safely from tags, release branches, repository conventions, or the user's wording.

Before asking the user anything, derive the release plan:

- `release_type`: from user wording or semver shape.
- `version`: from the explicit user version, otherwise from existing tags.
- `release_branch`: use the existing matching release branch when present; otherwise infer from recent branch convention. In this repository, use `release/<major>.<minor>.x`, for example `release/7.2.x` for `7.2.2`.
- `source`: user-provided source, otherwise the repository default branch.
- `tag`: repository's existing release tag format. In this repository, use the bare version as a signed annotated tag with message `Release v<version>`, for example `git tag -s 7.2.2 -m "Release v7.2.2"`.
- `push_mode`: local-only unless the user explicitly asks to push.
- `worktree_mode`: prefer temporary git worktrees when handling multiple release trains or when the current checkout is detached or managed by `jj`.

Ask a narrow question only when a field cannot be inferred safely or the missing answer would change release artifacts or risk pushing/tagging the wrong thing:

1. Release type, if not clear from the request or version number.
2. Exact version, if it cannot be inferred from existing tags.
3. Source branch, if it is not the default branch.
4. Commit selection, only if the user has not provided commits/PRs and has not asked you to inspect history and decide.
5. Whether to push, only after local branches/tags are ready.

For version inference:

- Inspect existing tags with `git tag --list --sort=-v:refname`.
- Inspect release branches with `git branch -a --list '*release*'`.
- For patch releases, increment the latest patch tag on the target release train.
- For minor and major releases, increment the requested component from the latest released version and reset lower components to zero.
- "One minor back" means the previous minor release branch; increment the latest patch tag on that line.
- Confirm inferred versions in progress updates, but keep working unless the user asked for confirmation before mutation.

## Hard stops

Stop and ask before continuing if:

- The working tree is dirty in a way not caused by this release work.
- A release branch or tag would be pushed without explicit approval.
- A force-push or tag replacement would be needed.
- Trivy or required tests fail before tagging or pushing.
- Cherry-pick conflicts remain unresolved.
- The target branch for an existing patch train is missing.

Never tag with a dirty working tree, unresolved conflicts, failed validation, or while checked out on the wrong branch.

## Validation

Before starting the release work:

1. Make sure the working tree is clean with `git status`.
2. Fetch current refs and tags with `git fetch --all --tags --prune`.
3. Run Trivy before creating final release artifacts, and earlier when the release is vulnerability-driven.
   - For this repository, use `mise x trivy -- trivy fs --exit-code 1 --severity HIGH,CRITICAL`.
   - Use a different documented Trivy command only if the repository adds one.
   - If running from a temporary worktree, `mise` may require trusting that worktree's `mise.toml`. Run `mise trust -y <worktree>/mise.toml` when needed, then remove or untrust temporary state during cleanup.

This repository is commonly used with `jj`, so `git status` may report `HEAD (no branch)`. Treat that as normal if the worktree is otherwise clean.

## Execute the release

Use the same workflow for patch, minor, and major releases. The release type mainly changes which commits from the source branch are included.

1. Inspect refreshed refs/tags and existing release branches.
2. Infer and state the release plan.
3. Validate the source and target as required.
4. Create or check out the target release branch:
   - For a new release line, create the release branch from the source branch.
   - For an existing release line, use the existing release branch.
5. Apply the release-type commit selection rules; use `git cherry-pick -x <sha>` for commits ported onto an existing release branch.
6. Resolve conflicts carefully and keep the release branch limited to selected commits and release-process changes.
7. Validate every release branch after porting changes.
8. Before tagging, show the release contents summary and ask `Proceed?`.
9. Tag locally on the release branch.
10. Push the release branch and tag only when requested or required by the release process and approved by the user.

Use this pre-tag summary format:

```md
### Included in <release>
- <one-line summary> - PR #<number>

### Excluded
- <one-line summary> - PR #<number> (reason: <reason>)

Proceed?
```

## Commit selection by release type

For patch releases, infer the selection from the purpose of a patch release: include only low-risk fixes needed for the release line, and exclude features, broad refactors, cleanup-only work, and unrelated dependency churn unless the user explicitly asks otherwise.

For vulnerability-driven patch releases, include the full remediation closure, not just commits whose messages mention a CVE, scanner, or vulnerability. Toolchain, runtime, base-image, dependency-manifest, lockfile, scanner-configuration, and build/release-definition changes may be required prerequisites even when their commit messages are generic.

For minor and major releases, include the normal release-train contents from the source branch, respecting any user-requested exclusions or intentionally breaking-change boundaries.

When the user asks you to inspect history and decide what to port, compare the target release branch to the source branch and inspect likely candidates:

- `git log --oneline --reverse origin/release/7.2.x..origin/main`
- `git show --stat --patch <candidate>`
- `git cherry -v origin/release/7.2.x <candidate>` to notice already-applied equivalent changes

Do not rely on commit messages alone. Inspect nearby commits, parent/child relationships, and file-level dependencies so prerequisite fixes are not missed. Before finalizing candidate selection, summarize any excluded adjacent prerequisite commits and why they are excluded.

## Tagging rules

- Always tag on the release branch.
- State the inferred tag before creating it; ask only if the format cannot be inferred or the operation will push.
- Prefer the repository's existing tag format. Inspect existing tags if unsure.
- Verify the checked-out branch before tagging with `git branch --show-current`.
- After tagging, show the created tag and the commit it points to.
- Summarize every mutation: branch creation, cherry-picks, version changes, commits, pushes, and tags.

## Local-only release summary

When local release artifacts are ready, report:

- Versions created and target branches.
- Cherry-picked source commits and resulting branch-tip commits.
- Tag names, tag targets, and whether tags are annotated/signed.
- Validation commands and outcomes.
- Ahead counts versus origin, for example `git rev-list --left-right --count origin/release/7.2.x...release/7.2.x`.
- That no pushes were performed, unless pushes were explicitly requested.
