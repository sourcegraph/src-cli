---
name: writing-prs-and-commits
description: Writes and revises Sourcegraph pull request titles, descriptions, and single-PR commit messages. Use when creating or updating a pull request, or when preparing a commit that should stand on its own as the eventual PR.
---

# Writing PRs and commits

Use this skill when the user asks to create a PR, update an existing PR title or body, or write a commit message for a change that maps cleanly to one PR.

## Resolve the target

- If the user names a PR number or URL, use that.
- Otherwise, inspect the current branch and use `gh pr view --json number,title,body,baseRefName,headRefName,url` to find the associated PR.
- If no PR exists and the user wants one created, use `gh pr create`, then immediately verify or adjust the title and body.
- If the user wants text only, provide the draft without mutating GitHub.
- When a PR is part of a stack, describe only the net change between the PR head and its base branch. Do not describe the full stack relative to `main`.

## Core writing rules

- Lead with _why_. The first paragraph or sentence should explain the problem, motivation, or user impact.
- Follow with _what_ changed. Once the reader understands why the change exists, describe the solution and any important constraints or tradeoffs.
- Write the net change only. Do not document abandoned experiments, reverted approaches, or temporary dead ends.
- Prefer natural prose over rigid `Summary` / `Problem` / `Solution` headings unless the user explicitly asks for that format.
- Keep references repo-relative. Do not mention absolute local filesystem paths.
- Preserve valuable existing PR content such as screenshots, images, rollout notes, and manually written context unless the user asks to remove it.
- Use Markdown cleanly: inline code in backticks, fenced blocks when needed, and GitHub permalinks when citing specific code.

## PR titles

- Keep titles short, specific, and verb-first.
- Prefer Sourcegraph's changelog-friendly shape when it fits the change: `type/domain: title`.
- Use conventional prefixes like `feat`, `fix`, `remove`, and `chore` when they match the change.
- Make the title describe the improvement or behavior change, not just the bug report.
- If the PR is user-visible and likely to feed the changelog, choose a domain that reflects the product or feature area rather than an implementation detail.

## PR descriptions

- Write the opening prose in this order:
  1. Why this change is needed.
  2. What changed.
  3. Any important tradeoffs, rollout notes, or follow-up context.
- When relevant, reference linked Linear issues or related Slack threads.
- If docs or handbook follow-up is needed, mention it in the prose before the verification section.
- Include `## Test Plan` near the end when there is meaningful validation to report.
- The test plan should mention intentional verification such as focused tests, manual flows, or reproduction steps. Do not pad it with routine formatting or CI work the repository already expects.
- Include `## Changelog` only for end-user-visible changes worth communicating outside the PR. Keep it as the last section. Omit it for refactors, tooling, internal cleanup, or other changes with no user-facing impact.

## Commit messages for single-PR changes

Treat a single commit that is intended to become one PR as a PR. GitHub by default will use the title and body from the commit.

## Workflow

1. Inspect the diff, linked context, and any existing PR title, body, or commit message.
2. Decide whether you are creating a new PR, editing an existing PR, or writing a stand-alone commit message.
3. Draft the title and body so the motivation is clear before the implementation details.
4. Check that the text matches the net change, preserves important existing content, and follows Sourcegraph conventions for `Test Plan` and `Changelog`.
5. Apply the final text with `gh pr create`, `gh pr edit`, or `git commit` as requested.
