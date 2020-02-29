package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
)

var verboseRunner bool

func init() {
	usage := `'src actions runner' TBD.

Usage:

	src actions runner [command options]
`

	flagSet := flag.NewFlagSet("runner", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src actions %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	// todo parse
	verboseRunner = false

	handler := func(args []string) error {
		r := &runner{}
		err := r.startRunner(4)
		return err
	}

	// Register the command.
	actionsCommands = append(actionsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

type RunnerConfig struct {
	runnerID string `json:"runnerId"`
}

type runner struct {
	client *client.Client
	conf   RunnerConfig
}

type envKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type actionDefinition struct {
	Steps           string `json:"steps"`
	ActionWorkspace struct {
		Name string `json:"name"`
	} `json:"actionWorkspace"`
	Env []envKV `json:"env"`
}

type actionJob struct {
	ID         string           `json:"id"`
	Definition actionDefinition `json:"definition"`
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
	BaseRevision string `json:"baseRevision"`
}

func (r *runner) startRunner(parallelJobCount int) error {
	ctx := context.Background()
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	r.conf.runnerID = "runner123"

	if err := r.createClient(); err != nil {
		return err
	}

	// todo: reenable
	// if err := cleanupOldContainers(ctx, r.client); err != nil {
	// 	return err
	// }

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		sig := <-sigs
		fmt.Println("Received signal", sig)
		cancelRun()
		r.stopAllJobs(ctx)
		if err := cleanupOldContainers(ctx, r.client); err != nil {
			// todo: err chan
			println(err.Error())
		}
		wg.Done()
	}()
	for i := 0; i < parallelJobCount; i++ {
		wg.Add(1)
		go func() {
			for {
				j, err := r.checkForJobs(runCtx)
				if err != nil {
					// todo: channel err
					// or maybe this can be ignored, at least N times
					// panic(err)
				} else if j != nil {
					fmt.Printf("Starting new job with ID %s\n", j.ID)
					diff, err := r.runActionJob(runCtx, j)
					if err != nil {
						println(err.Error())
						updateState(j, updateStateProps{status: "ERRORED"})
					} else {
						updatedState := updateStateProps{
							status: "COMPLETED",
						}
						// todo: nil pointer?
						if *diff != "" {
							updatedState.patch = diff
						}
						updateState(j, updatedState)
					}
				}
				wg.Done()
				// todo: revert to longer poll, with concurrency 1 it just holds
				time.Sleep(time.Second * 1)
				wg.Add(1)
			}
		}()
	}
	// wait for completion of signal handler
	wg.Wait()
	return nil
}

// creates the docker client
func (r *runner) createClient() error {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	r.client = c
	return nil
}

func (r *runner) stopAllJobs(ctx context.Context) error {
	// for _, job := range r.runningJobs {
	// 	if job != nil {
	// 		r.killContainer(ctx, job.container)
	// 	}
	// }
	return nil
}

func (r *runner) runActionJob(_ctx context.Context, job *actionJob) (*string, error) {
	_logBuffer := bytes.NewBuffer([]byte{})
	var logBuffer io.Writer
	if verboseRunner == true {
		logBuffer = io.MultiWriter(_logBuffer, os.Stdout)
	} else {
		logBuffer = _logBuffer
	}
	logBuffer.Write([]byte(fmt.Sprintln("Starting job")))
	runCtx, cancel := context.WithCancel(_ctx)
	defer cancel()

	// create periodic cancel checker
	go func() {
		for {
			isCanceled, err := checkIsCanceled(job)
			if err != nil {
				// todo:
				// attachCh <- err
				// break
			}
			if isCanceled == true {
				cancel()
				return
			}
			select {
			case <-runCtx.Done():
				return
			case <-time.After(time.Second * 2):
			}
		}
	}()

	// create periodic log streamer
	go func() {
		for {
			content := _logBuffer.String()
			_logBuffer.Reset()
			// todo: buffer might be written to during read, need concurrency lock
			 err := appendLog(job, content)
			if err != nil {
				// todo:
				// attachCh <- err
				// break
				println(err.Error())
			}
			select {
			case <-runCtx.Done():
				if err := appendLog(job, _logBuffer.String()); err != nil {
					// todo:
					// attachCh <- err
				}
				return
			case <-time.After(time.Second * 2):
			}
		}
	}()

	logBuffer.Write([]byte(fmt.Sprintln("Preparing execution context..")))

	x := executionContext{}
	err := x.prepare(runCtx, job.Repository.Name, job.BaseRevision, "test")
	defer x.cleanup()
	if err != nil {
		logBuffer.Write([]byte(fmt.Sprintf("Failed to prepare execution context: %s\n", err.Error())))
		return nil, errors.Wrap(err, "Failed to prepare execution context")
	}

	logBuffer.Write([]byte(fmt.Sprintln("Execution context set up")))

	var action Action
	if err := jsonxUnmarshal(string(job.Definition.Steps), &action); err != nil {
		logBuffer.Write([]byte(fmt.Sprintf("Invalid JSON action file: %s\n", err.Error())))
		return nil, errors.Wrap(err, "invalid JSON action file")
	}

	if err := validateAction(runCtx, action); err != nil {
		logBuffer.Write([]byte(fmt.Sprintf("Failed to validate action: %s\n", err.Error())))
		return nil, errors.Wrap(err, "Validation of action failed")
	}

	// Build Docker images etc.
	if err := prepareAction(runCtx, action); err != nil {
		logBuffer.Write([]byte(err.Error()))
		return nil, errors.Wrap(err, "Failed to prepare action")
	}

	for _, step := range action.Steps {
		if err := r.runContainer(runCtx, job, step, x.volumeDir, logBuffer); err != nil {
			logBuffer.Write([]byte(fmt.Sprintf("Container failed: %s\n", err.Error())))
			return nil, errors.Wrap(err, "Container failed")
		}
	}

	logBuffer.Write([]byte("Computing diff.."))
	d, err := computeDiff(runCtx, x.zipFile, x.volumeDir, "test")
	if err != nil {
		logBuffer.Write([]byte(fmt.Sprintf("Failed to compute diff: %s\n", err.Error())))
		return nil, errors.Wrap(err, "failed to compute diff")
	}
	logBuffer.Write([]byte("Done."))
	diffString := string(d)
	return &diffString, nil
}

func (r *runner) killContainer(ctx context.Context, cID string) error {
	return r.client.ContainerKill(ctx, cID, "SIGKILL")
}

func (r *runner) runContainer(ctx context.Context, job *actionJob, step *ActionStep, volumeDir string, log io.Writer) error {
	var image string
	if step.Type == "command" {
		// use ubuntu for command type for now
		image = "ubuntu"
	} else {
		image = step.Image
	}

	r.pullImage(ctx, image, log)

	// Set digests for Docker images so we don't cache action runs in 2 different images with
	// the same tag.
	if step.Image != "" {
		var err error
		step.ImageContentDigest, err = getDockerImageContentDigest(ctx, step.Image)
		if err != nil {
			return errors.Wrap(err, "Failed to get Docker image content digest")
		}
	}

	// generate env
	env := make([]string, len(job.Definition.Env))
	for i, kv := range job.Definition.Env {
		env[i] = fmt.Sprintf("%s=%s", kv.Key, kv.Value)
	}

	workDir := "/work"

	c, err := r.client.ContainerCreate(ctx, &container.Config{
		Image:        image,
		Cmd:          step.Args,
		Labels:       map[string]string{"com.sourcegraph.runner": "true"},
		Env:          env,
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   workDir,
	}, &container.HostConfig{
		Binds:         []string{fmt.Sprintf("%s:%s", volumeDir, workDir)},
		RestartPolicy: container.RestartPolicy{Name: "no"},
	}, nil, "") //, "container_name")

	if err != nil {
		return err
	}

	hij, err := r.client.ContainerAttach(ctx, c.ID, types.ContainerAttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	defer hij.Close()
	if err != nil {
		return err
	}

	attachCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	// create asynchronous log forwarder
	go func() {
		_, err := stdcopy.StdCopy(log, log, hij.Reader)
		if err != nil {
			fmt.Printf("Log stream error: %s", err.Error())
			attachCh <- err
		}
		wg.Done()
	}()

	err = r.client.ContainerStart(ctx, c.ID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	waitCh, errCh := r.client.ContainerWait(ctx, c.ID, container.WaitConditionNotRunning)
	select {
	case <-ctx.Done():
		println("container kill: ctx.Done")
		// todo: this context is already done
		// r.killContainer(ctx, c.ID)
		return errors.New("Context aborted")

	case err = <-attachCh:
		// log stream finished or errored
		println("container kill: attachCh")
		// r.killContainer(ctx, c.ID)
		if err != nil {
			return errors.Wrap(err, "")
		}

	case res := <-waitCh:
		// container has exited
		if res.StatusCode != 0 || res.Error != nil {
			// log job has failed
			log.Write([]byte(fmt.Sprintf("Container failed with status code %d\n", res.StatusCode)))
			if res.Error != nil {
				return errors.New(res.Error.Message)
			}
			return errors.New("Unknown failure on container")
		}
		log.Write([]byte("Container exited with 0 status code :-)\n"))
		// todo: do this wait everywhere
		wg.Wait()
		return nil

	case err = <-errCh:
		if err != nil {
			println("errCh")
			return err
		}
		return nil
	}

	return nil
}

func (r *runner) pullImage(ctx context.Context, image string, log io.Writer) error {
	log.Write([]byte(fmt.Sprintf("Pulling image %s\n", image)))
	logReader, err := r.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %s", image, err.Error())
	}
	// add logs from docker pull to the job log
	io.Copy(log, logReader)
	return nil
}

// todo: this kills containers from other runners as well, need to have some locking mechanism
func cleanupOldContainers(ctx context.Context, c *client.Client) error {
	// todo: detect that only one instance of the runner is running at a time,
	// otherwise they can steal the containers from each other
	println("Clearing up orphaned runner containers")
	containers, err := c.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "label", Value: "com.sourcegraph.runner=true"}),
	})
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	for _, cntr := range containers {
		wg.Add(1)
		cID := cntr.ID
		go func() {
			fmt.Printf("Stopping orphaned container %s\n", cID)
			if err := c.ContainerKill(ctx, cID, "SIGKILL"); err != nil {
				errCh <- err
			}
			if err = c.ContainerRemove(ctx, cID, types.ContainerRemoveOptions{Force: true, RemoveLinks: true, RemoveVolumes: true}); err != nil {
				errCh <- err
			}
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		errCh <- nil
	}()
	select {
	case err = <-errCh:
		if err != nil {
			return err
		}
		println("Done clearing up")
		return nil
	}
}

func (r *runner) checkForJobs(ctx context.Context) (*actionJob, error) {
	println("Checking for new jobs..")
	var result struct {
		PullActionJob *actionJob `json:"pullActionJob,omitempty"`
	}
	query := `mutation PullActionJob($runner: ID!) {
	pullActionJob(runner: $runner) {
		id
		definition {
			steps
			env {
				key
				value
			}
			actionWorkspace {
				name
			}
		}
		repository {
			name
		}
		baseRevision
	}
}`
	if err := (&apiRequest{
		query: query,
		vars: map[string]interface{}{
			"runner": r.conf.runnerID,
		},
		result: &result,
	}).do(); err != nil {
		println(err.Error())
		return nil, err
	}
	if result.PullActionJob != nil {
		fmt.Printf("Got job with ID '%s'\n", result.PullActionJob.ID)
		return result.PullActionJob, nil
	}
	return nil, nil
}

func appendLog(job *actionJob, content string) error {
	// todo: better chunking required
	if content == "" {
		return nil
	}
	var result struct{}
	query := `mutation AppendLog($actionJob: ID!, $content: String!) {
	appendLog(actionJob: $actionJob, content: $content) {
		id
	}
}`
	if err := (&apiRequest{
		query: query,
		vars: map[string]interface{}{
			"actionJob": job.ID,
			"content":   content,
		},
		result: &result,
	}).do(); err != nil {
		println(err.Error())
		return err
	}
	return  nil
}

type updateStateProps struct {
	status string
	patch  *string
}

func updateState(job *actionJob, state updateStateProps) error {
	if state.status != "" {
		if state.status == "PULLING" || state.status == "PREPARING" || state.status == "CREATING" {
			return nil
		}
		fmt.Printf("Status of container changed to %s\n", state.status)
	}
	if state.status == "" && state.patch == nil {
		// nothing to do
		return nil
	}
	var result struct{}
	query := `mutation UpdateActionJob($actionJob: ID!, $state: ActionJobState, $patch: String) {
	updateActionJob(actionJob: $actionJob, state: $state, patch: $patch) {
		id
	}
}`
	vars := map[string]interface{}{
		"actionJob": job.ID,
	}
	if state.status != "" {
		vars["state"] = state.status
	}
	if state.patch != nil {
		vars["patch"] = *state.patch
	}
	if err := (&apiRequest{
		query:  query,
		vars:   vars,
		result: &result,
	}).do(); err != nil {
		println(err.Error())
		return err
	}
	return nil
}


func checkIsCanceled(job *actionJob) (bool, error) {
	var result struct{
		Node *struct {
			State string `json:"state"`
		} `json:"node"`
	}
	query := `query ActionJobByID($id: ID!) {
	node(id: $id) {
		... on ActionJob {
			id
			state
		}
	}
}`
	if err := (&apiRequest{
		query: query,
		vars: map[string]interface{}{
			"id": job.ID,
		},
		result: &result,
	}).do(); err != nil {
		println(err.Error())
		return false, err
	}
	if result.Node == nil {
		// cancel non existant jobs
		return true, nil
	}
	return result.Node.State == "CANCELED", nil 
}