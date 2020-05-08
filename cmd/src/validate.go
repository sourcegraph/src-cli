package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/starlight-go/starlight"
)

type validator struct{}

func init() {
	usage := `'src validate' is a tool that validates a Sourcegraph instance.

EXPERIMENTAL: 'validate' is an experimental command in the 'src' tool.

Usage:

	src validate <script-file.star>

Examples:

  Validate the Sourcegraph instance with the starlark script: 'validate.star'

    	$ src validate validate.star
`
	flagSet := flag.NewFlagSet("validate", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src validate %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	vd := &validator{}

	commands = append(commands, &command{
		flagSet: flagSet,
		handler: func(args []string) error {
			if len(args) != 1 {
				return errors.New("no script argument")
			}
			return vd.validate(args[0])
		},
		usageFunc: usageFunc,
	})
}

func (vd *validator) validate(path string) error {
	script, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	globals := map[string]interface{}{
		"sg_add_external_service": vd.addExternalService,
		"sg_search": vd.searchMatchCount,
		"sg_repo_cloned": vd.repoCloned,
		"sg_sleep_seconds": vd.sleepSeconds,
	}

	vals, err := starlight.Eval(script, globals, nil)

	if passed, ok := vals["passed"].(bool); !ok || !passed {
		return errors.New("failed")
	}
	return nil
}

func (vd *validator) sleepSeconds(num time.Duration) {
	time.Sleep(time.Second * num)
}

const vdAddExternalServiceQuery = `
mutation AddExternalService($kind: ExternalServiceKind!, $displayName: String!, $config: String!) {
  addExternalService(input:{
    kind:$kind,
    displayName:$displayName,
    config: $config
  })
  {
    id
  }
}
`
func (vd *validator) addExternalService(kind, displayName, config string) (string, error) {
	var resp struct {
		Data struct {
			AddExternalService struct {
				ID string `json:"id"`
			} `json:"addExternalService"`
		} `json:"data"`
	}
	err := (&apiRequest{
		query: vdAddExternalServiceQuery,
		vars: map[string]interface{}{
			"kind": kind,
			"displayName": displayName,
			"config": config,
		},
		result: &resp,
	}).do()

	return resp.Data.AddExternalService.ID, err
}

const vdSearchQuery = `
query ($query: String!) {
  search(query: $query, version: V2, patternType:literal) {
    results {
      matchCount
    }
  }
}`

func (vd *validator) searchMatchCount(searchStr string) (int, error) {
	var resp struct {
		Search struct {
			Results struct {
				MatchCount int `json:"matchCount"`
			} `json:"results"`
		} `json:"search"`
	}

	err := (&apiRequest{
		query: vdSearchQuery,
		vars: map[string]interface{}{
			"query": searchStr,
		},
		result: &resp,
	}).do()

	return resp.Search.Results.MatchCount, err
}

const vdRepoCloning = `
query RepoCloning($repoName:String!) {
  repositories(
    cloneInProgress: false,
    cloned: true,
    notCloned: false,
    names:[$repoName],
  ) {
    nodes {
      name
    }
  }
}`

func (vd *validator) repoCloned(repoName string) (bool, error) {
	var resp struct {
		Repositories struct {
			Nodes []struct {
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"repositories"`
	}

	err := (&apiRequest{
		query: vdRepoCloning,
		vars: map[string]interface{}{
			"repoName": repoName,
		},
		result: &resp,
	}).do()

	return len(resp.Repositories.Nodes) == 1, err
}
