// Code generated by stringdata. DO NOT EDIT.

package schema

// CampaignSpecJSON is the content of the file "campaign_spec.schema.json".
const CampaignSpecJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "CampaignSpec",
  "description": "A campaign specification, which describes the campaign and what kinds of changes to make (or what existing changesets to track).",
  "type": "object",
  "additionalProperties": false,
  "required": ["name"],
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the campaign, which is unique among all campaigns in the namespace. A campaign's name is case-preserving.",
      "pattern": "^[\\w.-]+$"
    },
    "description": {
      "type": "string",
      "description": "The description of the campaign."
    },
    "on": {
      "type": "array",
      "description": "The set of repositories (and branches) to run the campaign on, specified as a list of search queries (that match repositories) and/or specific repositories.",
      "items": {
        "title": "OnQueryOrRepository",
        "oneOf": [
          {
            "title": "OnQuery",
            "type": "object",
            "description": "A Sourcegraph search query that matches a set of repositories (and branches). Each matched repository branch is added to the list of repositories that the campaign will be run on.",
            "additionalProperties": false,
            "required": ["repositoriesMatchingQuery"],
            "properties": {
              "repositoriesMatchingQuery": {
                "type": "string",
                "description": "A Sourcegraph search query that matches a set of repositories (and branches). If the query matches files, symbols, or some other object inside a repository, the object's repository is included.",
                "examples": ["file:README.md"]
              }
            }
          },
          {
            "title": "OnRepository",
            "type": "object",
            "description": "A specific repository (and branch) that is added to the list of repositories that the campaign will be run on.",
            "additionalProperties": false,
            "required": ["repository"],
            "properties": {
              "repository": {
                "type": "string",
                "description": "The name of the repository (as it is known to Sourcegraph).",
                "examples": ["github.com/foo/bar"]
              },
              "branch": {
                "type": "string",
                "description": "The branch on the repository to propose changes to. If unset, the repository's default branch is used."
              }
            }
          }
        ]
      }
    },
    "steps": {
      "type": "array",
      "description": "The sequence of commands to run (for each repository branch matched in the ` + "`" + `on` + "`" + ` property) to produce the campaign's changes.",
      "items": {
        "title": "Step",
        "type": "object",
        "description": "A command to run (as part of a sequence) in a repository branch to produce the campaign's changes.",
        "additionalProperties": false,
        "required": ["run", "container"],
        "properties": {
          "run": {
            "type": "string",
            "description": "The shell command to run in the container. It can also be a multi-line shell script. The working directory is the root directory of the repository checkout."
          },
          "container": {
            "type": "string",
            "description": "The Docker image used to launch the Docker container in which the shell command is run.",
            "examples": ["alpine:3"]
          },
          "env": {
            "description": "Environment variables to set in the step environment.",
            "oneOf": [
              {
                "type": "object",
                "description": "Environment variables to set in the step environment.",
                "additionalProperties": {
                  "type": "string"
                }
              },
              {
                "type": "array",
                "items": {
                  "oneOf": [
                    {
                      "type": "string",
                      "description": "An environment variable to set in the step environment: the value will be passed through from the environment src is running within."
                    },
                    {
                      "type": "object",
                      "description": "An environment variable to set in the step environment: the value will be passed through from the environment src is running within.",
                      "additionalProperties": { "type": "string" },
                      "minProperties": 1,
                      "maxProperties": 1
                    }
                  ]
                }
              }
            ]
          },
          "files": {
            "type": "object",
            "description": "Files that should be mounted into or be created inside the Docker container.",
            "additionalProperties": {"type": "string"}
          }
        }
      }
    },
    "transformChanges": {
      "type": "object",
      "description": "Optional transformations to apply to the changes produced in each repository.",
      "additionalProperties": false,
      "properties": {
        "group": {
          "type": "array",
          "description": "A list of groups of changes in a repository that each create a separate, additional changeset for this repository, with all ungrouped changes being in the default changeset.",
          "additionalProperties": false,
          "required": ["directory", "branchSuffix"],
          "properties": {
            "directory": {
              "type": "string",
              "description": "The directory path (relative to the repository root) of the changes to include in this group."
            },
            "branchSuffix": {
              "type": "string",
              "description": "The branch suffix to add to the ` + "`" + `branch` + "`" + ` attribute of the ` + "`" + `changesetTemplate` + "`" + ` when creating the additonal changeset."
            }
          }
        }
      }
    },
    "importChangesets": {
      "type": "array",
      "description": "Import existing changesets on code hosts.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["repository", "externalIDs"],
        "properties": {
          "repository": {
            "type": "string",
            "description": "The repository name as configured on your Sourcegraph instance."
          },
          "externalIDs": {
            "type": "array",
            "description": "The changesets to import from the code host. For GitHub this is the PR number, for GitLab this is the MR number, for Bitbucket Server this is the PR number.",
            "uniqueItems": true,
            "items": {
              "oneOf": [{ "type": "string" }, { "type": "integer" }]
            },
            "examples": [120, "120"]
          }
        }
      }
    },
    "changesetTemplate": {
      "type": "object",
      "description": "A template describing how to create (and update) changesets with the file changes produced by the command steps.",
      "additionalProperties": false,
      "required": ["title", "branch", "commit", "published"],
      "properties": {
        "title": { "type": "string", "description": "The title of the changeset." },
        "body": { "type": "string", "description": "The body (description) of the changeset." },
        "branch": {
          "type": "string",
          "description": "The name of the Git branch to create or update on each repository with the changes."
        },
        "commit": {
          "title": "ExpandedGitCommitDescription",
          "type": "object",
          "description": "The Git commit to create with the changes.",
          "additionalProperties": false,
          "required": ["message"],
          "properties": {
            "message": {
              "type": "string",
              "description": "The Git commit message."
            },
            "author": {
              "title": "GitCommitAuthor",
              "type": "object",
              "description": "The author of the Git commit.",
              "additionalProperties": false,
              "required": ["name", "email"],
              "properties": {
                "name": {
                  "type": "string",
                  "description": "The Git commit author name."
                },
                "email": {
                  "type": "string",
                  "format": "email",
                  "description": "The Git commit author email."
                }
              }
            }
          }
        },
        "published": {
          "description": "Whether to publish the changeset. An unpublished changeset can be previewed on Sourcegraph by any person who can view the campaign, but its commit, branch, and pull request aren't created on the code host. A published changeset results in a commit, branch, and pull request being created on the code host.",
          "oneOf": [
            {
              "oneOf": [{ "type": "boolean" }, { "type": "string", "pattern": "^draft$" }],
              "description": "A single flag to control the publishing state for the entire campaign."
            },
            {
              "type": "array",
              "description": "A list of glob patterns to match repository names. In the event multiple patterns match, the last matching pattern in the list will be used.",
              "items": {
                "type": "object",
                "description": "An object with one field: the key is the glob pattern to match against repository names; the value will be used as the published flag for matching repositories.",
                "additionalProperties": { "oneOf": [{ "type": "boolean" }, { "type": "string", "pattern": "^draft$" }] },
                "minProperties": 1,
                "maxProperties": 1
              }
            }
          ]
        }
      }
    }
  }
}
`
