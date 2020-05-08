package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/starlight-go/starlight"
	"github.com/starlight-go/starlight/convert"
	"go.starlark.net/starlark"
)

type validator struct{}

func init() {
	usage := `'src validate' is a tool that validates a Sourcegraph instance.

EXPERIMENTAL: 'validate' is an experimental command in the 'src' tool.

Usage:

	src validate [options] <script-file>
or
    cat <script-file> | src validate [options]
`
	flagSet := flag.NewFlagSet("validate", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src validate %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		contextFlag = flagSet.String("context", "", `Comma-separated list of key=value pairs to add to the script execution context`)
		docFlag = flagSet.Bool("doc", false, `Show function documentation`)
	)

	vd := &validator{}

	commands = append(commands, &command{
		flagSet: flagSet,
		handler: func(args []string) error {
			if *docFlag {
				vd.printDocumentation()
				return nil
			}

			var script []byte
			var err error
			if len(flagSet.Args()) == 1 {
				script, err = ioutil.ReadFile(flagSet.Arg(0))
				if err != nil {
					return err
				}
			}
			if !isatty.IsTerminal(os.Stdin.Fd()) {
				// stdin is a pipe not a terminal
				script, err = ioutil.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
			}

			return vd.validate(script, vd.parseScriptContext(*contextFlag))
		},
		usageFunc: usageFunc,
	})
}

func (vd *validator) printDocumentation() {
	fmt.Println("TODO(uwedeportivo): write function documentation")
}

func (vd *validator) parseScriptContext(val string) map[string]string {
	scriptContext := make(map[string]string)

	pairs := strings.Split(val, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")

		if len(kv) == 2 {
			scriptContext[kv[0]] = kv[1]
		}
	}
	return scriptContext
}

func (vd *validator) validate(script []byte, scriptContext map[string]string) error {
	globals := map[string]interface{}{
		"src_list_external_services": vd.listExternalServices,
		"src_add_external_service": vd.addExternalService,
		"src_delete_external_service": vd.deleteExternalService,
		"src_search_match_count": vd.searchMatchCount,
		"src_list_cloned_repos": vd.listClonedRepos,
		"src_sleep_seconds": vd.sleepSeconds,
		"src_log": vd.log,
		"src_run_graphql": vd.runGraphQL,
		"src_context": scriptContext,
	}

	vals, err := starlight.Eval(script, globals, nil)
	if err != nil {
		return err
	}
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
}`

func (vd *validator) addExternalService(kind, displayName, config interface{}) (string, error) {
	dict := vd.convertDict(config)
	configJson, err := json.MarshalIndent(dict, "", "  ")
	if err != nil {
		return "", err
	}
	var resp struct {
		AddExternalService struct {
			ID string `json:"id"`
		} `json:"addExternalService"`
	}
	err = (&apiRequest{
		query: vdAddExternalServiceQuery,
		vars: map[string]interface{}{
			"kind": kind,
			"displayName": displayName,
			"config": string(configJson),
		},
		result: &resp,
	}).do()

	return resp.AddExternalService.ID, err
}

const vdDeleteExternalServiceQuery = `
mutation DeleteExternalService($id: ID!) {
  deleteExternalService(externalService: $id){
    alwaysNil
  } 
}`

func (vd *validator) deleteExternalService(id string) error {
	var resp struct {}

	err := (&apiRequest{
		query: vdDeleteExternalServiceQuery,
		vars: map[string]interface{}{
			"id": id,
		},
		result: &resp,
	}).do()

	return err
}

const vdSearchMatchCountQuery = `
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
		query: vdSearchMatchCountQuery,
		vars: map[string]interface{}{
			"query": searchStr,
		},
		result: &resp,
	}).do()

	return resp.Search.Results.MatchCount, err
}

const vdListRepos = `
query ListRepos($cloneInProgress: Boolean!, $cloned: Boolean!, $notCloned: Boolean!, $names: [String!]) {
  repositories(
    cloned: $cloneInProgress
    cloneInProgress: $cloned
    notCloned: $notCloned
    names: $names
  ) {
    nodes {
      name
    }
  }
}`

func (vd *validator) listClonedRepos(filterNames interface{}) ([]string, error) {
	fs := vd.convertStringList(filterNames)
	var resp struct {
		Repositories struct {
			Nodes []struct {
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"repositories"`
	}

	err := (&apiRequest{
		query: vdListRepos,
		vars: map[string]interface{}{
			"cloneInProgress": false,
			"cloned": true,
			"notCloned": false,
			"names": fs,
		},
		result: &resp,
	}).do()

	names := make([]string, 0, len(resp.Repositories.Nodes))
	for _, node := range resp.Repositories.Nodes {
		names = append(names, node.Name)
	}

	return names, err
}

func (vd *validator) log(line string) {
	fmt.Println(line)
}

func (vd *validator) runGraphQL(query string, vars map[string]interface{}) (map[string]interface{}, error) {
	resp := map[string]interface{}{}

	err := (&apiRequest{
		query: query,
		vars: vars,
		result: &resp,
	}).do()

	return resp, err
}

const vdListExternalServices = `
query ExternalServices {
  externalServices {
    nodes {
      id
      displayName
    }
  }
}`

func (vd *validator) listExternalServices() ([]interface{}, error) {
	var resp struct {
		ExternalServices struct {
			Nodes []struct {
				DisplayName string `json:"displayName"`
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"externalServices"`
	}

	err := (&apiRequest{
		query: vdListExternalServices,
		result: &resp,
	}).do()

	xs := make([]interface{}, 0, len(resp.ExternalServices.Nodes))
	for _, es := range resp.ExternalServices.Nodes {
		xs = append(xs, map[string]string{"id": es.ID, "displayName": es.DisplayName})
	}

	return xs, err
}

func (vd *validator) convertDict(val interface{}) map[string]interface{} {
	dict := val.(map[interface{}]interface{})
	res := make(map[string]interface{})

	for k, v := range dict {
		gk := vd.fromStarLark(k).(string)
		gv := vd.fromStarLark(v)

		res[gk] = gv
	}
	return res
}

func (vd *validator) convertStringList(val interface{}) []string {
	list := val.([]interface{})
	res := make([]string, 0, len(list))

	for _, v := range list {
		gv := v.(string)

		res = append(res, gv)
	}
	return res
}

func (vd *validator) fromStarLark(v interface{}) interface{} {
	switch v := v.(type) {
	case starlark.Bool:
		return bool(v)
	case starlark.Int:
		// starlark ints can be signed or unsigned
		if i, ok := v.Int64(); ok {
			return i
		}
		if i, ok := v.Uint64(); ok {
			return i
		}
		// buh... maybe > maxint64?  Dunno
		panic(fmt.Errorf("can't convert starlark.Int %q to int", v))
	case starlark.Float:
		return float64(v)
	case starlark.String:
		return string(v)
	case *starlark.List:
		return convert.FromList(v)
	case starlark.Tuple:
		return convert.FromTuple(v)
	case *starlark.Dict:
		return convert.FromDict(v)
	case *starlark.Set:
		return convert.FromSet(v)
	default:
		// dunno, hope it's a custom type that the receiver knows how to deal
		// with. This can happen with custom-written go types that implement
		// starlark.Value.
		return v
	}
}
