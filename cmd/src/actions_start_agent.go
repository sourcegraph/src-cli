package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"gopkg.in/inconshreveable/log15.v2"
)

func init() {
	usage := `
Start the actions agent that listens for incoming requests to execute actions.

TODO
`

	flagSet := flag.NewFlagSet("start-agent", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src actions %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	cacheDir, _ := userCacheDir()
	if cacheDir != "" {
		cacheDir = filepath.Join(cacheDir, "action-exec")
	}

	displayUserCacheDir := strings.Replace(cacheDir, os.Getenv("HOME"), "$HOME", 1)

	var (
		portFlag        = flagSet.String("port", "8080", "The port on which to listen for requests.")
		parallelismFlag = flagSet.Int("j", runtime.GOMAXPROCS(0), "The number of parallel jobs.")
		cacheDirFlag    = flagSet.String("cache", displayUserCacheDir, "Directory for caching results.")
		keepLogsFlag    = flagSet.Bool("keep-logs", false, "Do not remove execution log files when done.")
		timeoutFlag     = flagSet.Duration("timeout", defaultTimeout, "The maximum duration a single action run can take (excluding the building of Docker images).")
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		if *cacheDirFlag == displayUserCacheDir {
			*cacheDirFlag = cacheDir
		}

		if *cacheDirFlag == "" {
			return errors.New("cache is not a valid path")
		}

		agent := &Agent{
			port:        *portFlag,
			parallelism: *parallelismFlag,
			keepLogs:    *keepLogsFlag,
			timeout:     *timeoutFlag,
			cacheDir:    *cacheDirFlag,

			executors:     make(map[uuid.UUID]*executorStatus),
			statusUpdates: make(chan statusUpdate),
		}

		go agent.ListenUpdates()

		handler := handlers.LoggingHandler(os.Stdout, agent.Handler())
		srv := &http.Server{Addr: ":" + *portFlag, Handler: handler}
		log15.Info("agent: listening", "addr", srv.Addr)

		go func() {
			err := srv.ListenAndServe()
			if err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}()

		c := make(chan os.Signal, 2)
		signal.Notify(c, syscall.SIGINT, syscall.SIGHUP)
		<-c
		go func() {
			<-c
			os.Exit(0)
		}()

		srv.Close()

		return nil
	}

	// Register the command.
	actionsCommands = append(actionsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

type executorStatus struct {
	done    bool
	repos   map[ActionRepo]ActionRepoStatus
	patches []CampaignPlanPatch
}

type statusUpdate struct {
	id uuid.UUID

	status *executorStatus
}

type Agent struct {
	port string

	parallelism int
	keepLogs    bool
	timeout     time.Duration
	cacheDir    string

	executors   map[uuid.UUID]*executorStatus
	executorsMu sync.Mutex

	statusUpdates chan statusUpdate
}

func (a *Agent) ListenUpdates() {
	for update := range a.statusUpdates {
		a.handleUpdate(update)
	}
}

func (a *Agent) handleUpdate(u statusUpdate) {
	a.executorsMu.Lock()
	defer a.executorsMu.Unlock()

	_, ok := a.executors[u.id]
	if !ok {
		return
	}

	a.executors[u.id] = u.status
}

func (a *Agent) Handler() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/exec", a.handleExec)
	r.HandleFunc("/progress/{id}", a.handleExecProgress)
	r.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

func (a *Agent) handleExec(w http.ResponseWriter, r *http.Request) {
	var action Action
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.execAction(w, r, action)
}

type remoteExecResponse struct {
	ID string `json:"id"`
}

func (a *Agent) execAction(w http.ResponseWriter, r *http.Request, action Action) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Build Docker images etc.
	err := prepareAction(ctx, action)
	if err != nil {
		respond(w, http.StatusInternalServerError, err)
		return
	}

	executorID, err := uuid.NewRandom()
	if err != nil {
		respond(w, http.StatusInternalServerError, err)
		return
	}

	onUpdate := func(repos map[ActionRepo]ActionRepoStatus) {
		a.statusUpdates <- statusUpdate{id: executorID, status: &executorStatus{repos: repos}}
	}

	executor := newActionExecutor(action, a.parallelism, actionExecutorOptions{
		timeout:  a.timeout,
		keepLogs: a.keepLogs,
		cache:    actionExecutionDiskCache{dir: a.cacheDir},

		onUpdate:         onUpdate,
		onUpdateInterval: 1 * time.Second,
	})

	// Query repos over which to run action
	repos, err := actionRepos(ctx, *verbose, action.ScopeQuery)
	if err != nil {
		respond(w, http.StatusInternalServerError, err)
		return
	}

	for _, repo := range repos {
		executor.enqueueRepo(repo)
	}

	a.executorsMu.Lock()
	a.executors[executorID] = &executorStatus{repos: executor.repos}
	a.executorsMu.Unlock()

	go executor.start(context.Background())
	log15.Info("Executor started", "id", executorID)

	go func() {
		if err := executor.wait(); err != nil {
			log15.Error("executor errored", "error", err)
		}

		log15.Info("Executor finished execution", "id", executorID)
		a.statusUpdates <- statusUpdate{
			id: executorID,
			status: &executorStatus{
				done:    true,
				repos:   executor.repos,
				patches: executor.allPatches(),
			},
		}
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&remoteExecResponse{
		ID: executorID.String(),
	})
}

type RepoProgress struct {
	Repo   ActionRepo
	Status ActionRepoStatus
}

type remoteExecProgressResponse struct {
	Done    bool                `json:"done"`
	Repos   []RepoProgress      `json:"repos"`
	Patches []CampaignPlanPatch `json:"patches"`
}

func (a *Agent) handleExecProgress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idVar, ok := vars["id"]
	if !ok {
		http.Error(w, "No ID given", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idVar)
	if err != nil {
		respond(w, http.StatusBadRequest, err)
		return
	}

	a.executorsMu.Lock()
	es, ok := a.executors[id]
	a.executorsMu.Unlock()

	if !ok {
		respond(w, http.StatusNotFound, fmt.Errorf("No executor with ID %s found", id))
		return
	}

	res := &remoteExecProgressResponse{
		Done:    es.done,
		Patches: es.patches,
	}
	for repo, status := range es.repos {
		res.Repos = append(res.Repos, RepoProgress{Repo: repo, Status: status})
	}

	respond(w, http.StatusOK, res)
}

func respond(w http.ResponseWriter, code int, v interface{}) {
	switch val := v.(type) {
	case error:
		if val != nil {
			log15.Error(val.Error())
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(code)
			fmt.Fprintf(w, "%v", val)
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		bs, err := json.Marshal(v)
		if err != nil {
			respond(w, http.StatusInternalServerError, err)
			return
		}

		w.WriteHeader(code)
		if _, err = w.Write(bs); err != nil {
			log15.Error("failed to write response", "error", err)
		}
	}
}
