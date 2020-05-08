package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/mattn/go-isatty"
	"github.com/starlight-go/starlight"
	"github.com/starlight-go/starlight/convert"
	"go.starlark.net/starlark"
)

type validator struct{
	client *vdClient
}

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
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src validate %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		contextFlag = flagSet.String("context", "", `Comma-separated list of key=value pairs to add to the script execution context`)
		docFlag = flagSet.Bool("doc", false, `Show function documentation`)
		secretsFlag = flagSet.String("secrets", "", "Path to a file containing key=value lines. The key value pairs will be added to the script context")
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

			ctxm := vd.parseKVPairs(*contextFlag, ",")

			if *secretsFlag != "" {
				sm, err := vd.readSecrets(*secretsFlag)
				if err != nil {
					return err
				}

				for k, v := range sm {
					ctxm[k] = v
				}
			}

			return vd.validate(script, ctxm)
		},
		usageFunc: usageFunc,
	})
}

func (vd *validator) printDocumentation() {
	fmt.Println("TODO(uwedeportivo): write function documentation")
}

func (vd *validator) parseKVPairs(val string, pairSep string) map[string]string {
	scriptContext := make(map[string]string)

	pairs := strings.Split(val, pairSep)
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")

		if len(kv) == 2 {
			scriptContext[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return scriptContext
}

func (vd *validator) readSecrets(path string) (map[string]string, error) {
	bs, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return vd.parseKVPairs(string(bs), "\n"), nil
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
		"src_create_first_admin": vd.createFirstAdmin,
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
	
	err = vd.graphQL(vdAddExternalServiceQuery, map[string]interface{}{
		"kind": kind,
		"displayName": displayName,
		"config": string(configJson),
	}, &resp)

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

// SiteAdminInit initializes the instance with given admin account.
// It returns an authenticated client as the admin for doing e2e testing.
func (vd *validator) siteAdminInit(baseURL, email, username, password string) (*vdClient, error) {
	client, err := vd.newClient(baseURL)
	if err != nil {
		return nil, err
	}

	var request = struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Email:    email,
		Username: username,
		Password: password,
	}
	err = client.authenticate("/-/site-init", request)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// SignIn performs the sign in with given user credentials.
// It returns an authenticated client as the user for doing e2e testing.
func (vd *validator) signIn(baseURL string, email, password string) (*vdClient, error) {
	client, err := vd.newClient(baseURL)
	if err != nil {
		return nil, err
	}

	var request = struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{
		Email:    email,
		Password: password,
	}
	err = client.authenticate("/-/sign-in", request)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// extractCSRFToken extracts CSRF token from HTML response body.
func (vd *validator) extractCSRFToken(body string) string {
	anchor := `X-Csrf-Token":"`
	i := strings.Index(body, anchor)
	if i == -1 {
		return ""
	}

	j := strings.Index(body[i+len(anchor):], `","`)
	if j == -1 {
		return ""
	}

	return body[i+len(anchor) : i+len(anchor)+j]
}

// Client is an authenticated client for a Sourcegraph user for doing e2e testing.
// The user may or may not be a site admin depends on how the client is instantiated.
// It works by simulating how the browser would send HTTP requests to the server.
type vdClient struct {
	baseURL       string
	csrfToken     string
	csrfCookie    *http.Cookie
	sessionCookie *http.Cookie

	userID string
}

// newClient instantiates a new client by performing a GET request then obtains the
// CSRF token and cookie from its response.
func (vd *validator) newClient(baseURL string) (*vdClient, error) {
	resp, err := http.Get(baseURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	p, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	csrfToken := vd.extractCSRFToken(string(p))
	if csrfToken == "" {
		return nil, err
	}
	var csrfCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "sg_csrf_token" {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil {
		return nil, errors.New(`"sg_csrf_token" cookie not found`)
	}

	return &vdClient{
		baseURL:    baseURL,
		csrfToken:  csrfToken,
		csrfCookie: csrfCookie,
	}, nil
}

// authenticate is used to send a HTTP POST request to an URL that is able to authenticate
// a user with given body (marshalled to JSON), e.g. site admin init, sign in. Once the
// client is authenticated, the session cookie will be stored as a proof of authentication.
func (c *vdClient) authenticate(path string, body interface{}) error {
	p, err := jsoniter.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(p))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Csrf-Token", c.csrfToken)
	req.AddCookie(c.csrfCookie)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(p))
	}

	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "sgs" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		return err
	}
	c.sessionCookie = sessionCookie

	userID, err := c.currentUserID()
	if err != nil {
		return err
	}
	c.userID = userID
	return nil
}

// currentUserID returns the current user's GraphQL node ID.
func (c *vdClient) currentUserID() (string, error) {
	query := `
	query {
		currentUser {
			id
		}
	}
`
	var resp struct {
		Data struct {
			CurrentUser struct {
				ID string `json:"id"`
			} `json:"currentUser"`
		} `json:"data"`
	}
	err := c.graphQL("", query, nil, &resp)
	if err != nil {
		return "", err
	}

	return resp.Data.CurrentUser.ID, nil
}

// GraphQL makes a GraphQL request to the server on behalf of the user authenticated by the client.
// An optional token can be passed to impersonate other users.
func (c *vdClient) graphQL(token, query string, variables map[string]interface{}, target interface{}) error {
	body, err := jsoniter.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/.api/graphql", c.baseURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	} else {
		// NOTE: We use this header to protect from CSRF attacks of HTTP API,
		// see https://sourcegraph.com/github.com/sourcegraph/sourcegraph/-/blob/cmd/frontend/internal/cli/http.go#L41-42
		req.Header.Set("X-Requested-With", "Sourcegraph")
		req.AddCookie(c.csrfCookie)
		req.AddCookie(c.sessionCookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(p))
	}

	return jsoniter.NewDecoder(resp.Body).Decode(target)
}

func (vd *validator) createFirstAdmin(email, username, password string) error {
	client, err := vd.signIn(cfg.Endpoint, email, password)
	if err != nil {
		client, err = vd.siteAdminInit(cfg.Endpoint, email, username, password)
		if err != nil {
			return err
		}
	}

	vd.client = client
	return nil
}

func (vd *validator) graphQL(query string, variables map[string]interface{}, target interface{}) error {
	if vd.client != nil {
		return vd.client.graphQL("", query, variables, target)
	}

	return (&apiRequest{
		query: vdListExternalServices,
		result: target,
	}).do()
}
