# `src mcp` Experiment

> **Status:** Development paused. The Sourcegraph MCP with DCR is preferred.

## Motivation

We added functionality to the `src` CLI to dynamically expose Sourcegraph MCP tools as CLI commands. The aim was that exposing these commands as skills to agents would help them interact with the Sourcegraph instance more easily and use that knowledge to perform their tasks locally.

## Findings

Once `src mcp` was exposed to agents with skills, the results were hit and miss.

* Some agents like Claude became overzealous in reaching out to Sourcegraph and would use `src mcp` over grepping the source code locally.
* In Amp, you really had to explicitly invoke `src mcp` — it wouldn't organically use it on its own. Sometimes it would, but that was the exception. The only time it would really pick `src mcp` as the first choice was if your query was extremely obvious that it should use Sourcegraph e.g. "Find all changes made by William across the Sourcegraph organization repositories".
* By having the Sourcegraph MCP *and* the `src mcp` skill, the agent picked the MCP all the time and under the right circumstances.
  * For example, when asked "When and why was gitserver refactored to use grpc?" with the MCP installed, the agent immediately reached for `sg_deepsearch`.
  * With only the `src mcp` skill, it never used deepsearch unless explicitly told — defaulting to keyword search and commit search instead.

Through my own use, there was no compelling reason to have `src mcp` over the normal Sourcegraph MCP, especially now that the Sourcegraph MCP supports DCR (dynamic client registration).

## Decision

We're stopping additional development on `src mcp` as the usefulness of the experiment is just not there yet. We might pick it up in the future but for now further investment is not worth it.

The command isn't advertised in help anywhere so there is little to no risk in stopping development. The only way to know the command exists is to read the code.

We will keep the code as it is dynamic and adapts to what the Sourcegraph instance exposes so maintenance should be low.
