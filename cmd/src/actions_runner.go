package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/sourcegraph/go-diff/diff"
)

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

	handler := func(args []string) error {
		r := &runner{}
		err := r.startRunner(2)
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
	runningJobs map[int]*jobRunner
	client      *client.Client
	conf        RunnerConfig
}

type jobRunner struct {
	actionJob actionJob
	container *string
	client    *client.Client
}

type envKV struct {
	key   string
	value string
}

type actionJob struct {
	ID    string
	image string
	env   []envKV
	diff  *string
}

func (r *runner) startRunner(parallelJobCount int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.runningJobs = make(map[int]*jobRunner, parallelJobCount)
	r.conf.runnerID = "runner123"

	if err := r.createClient(); err != nil {
		return err
	}

	if err := cleanupOldContainers(ctx, r.client); err != nil {
		return err
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		sig := <-sigs
		fmt.Println("Received signal", sig)
		r.stopAllJobs(ctx)
		if err := cleanupOldContainers(ctx, r.client); err != nil {
			// todo: err chan
			panic(err)
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		for {
			j, err := r.checkForJobs(ctx)
			if err != nil {
				// todo: channel err
				// or maybe this can be ignored, at least N times
				// panic(err)
			} else if j != nil {
				println("Starting new job")
				if err := r.runActionJob(ctx, j); err != nil {
					panic(err)
				}
				if j.diff != nil {
					println("Generated diff", *j.diff)
				} else {
					println("Resulted in no diff")
				}
			}
			wg.Done()
			time.Sleep(time.Second * 30)
			wg.Add(1)
		}
	}()
	// wait for completion of signal handler
	wg.Wait()
	return nil
}

func (r *runner) createClient() error {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	r.client = c
	return nil
}

func (r *runner) stopAllJobs(ctx context.Context) {
	for _, job := range r.runningJobs {
		if job != nil {
			stopRunner(ctx, job)
		}
	}
}

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

func stopRunner(ctx context.Context, r *jobRunner) {
	if r.container != nil {
		fmt.Printf("Killing container %s\n", *r.container)
		r.client.ContainerKill(ctx, *r.container, "SIGKILL")
	}
}

var lastJob int = 0

func (r *runner) runActionJob(_ctx context.Context, job *actionJob) error {
	ctx, cancel := context.WithCancel(_ctx)
	defer cancel()
	changeStatus(job, "PREPARING")

	zipFile, err := fetchRepositoryArchive(ctx, "github.com/sourcegraph/sourcegraph", "master")
	if err != nil {
		return err // errors.Wrap(err, "Fetching ZIP archive failed")
	}
	defer os.Remove(zipFile.Name())

	prefix := "action-" + strings.Replace(strings.Replace("github.com/sourcegraph/sourcegraph", "/", "-", -1), "github.com-", "", -1)
	volumeDir, err := unzipToTempDir(ctx, zipFile.Name(), prefix)
	if err != nil {
		return err // errors.Wrap(err, "Unzipping the ZIP archive failed")
	}
	defer os.RemoveAll(volumeDir)

	changeStatus(job, "CREATING")
	jr := &jobRunner{
		actionJob: *job,
		client:    r.client,
	}
	r.runningJobs[lastJob] = jr
	lastJob++
	changeStatus(job, "PULLING")
	reader, err := r.client.ImagePull(ctx, jr.actionJob.image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %s", jr.actionJob.image, err.Error())
	}
	io.Copy(os.Stdout, reader)
	// generate env
	env := make([]string, len(jr.actionJob.env))
	for i, kv := range jr.actionJob.env {
		env[i] = fmt.Sprintf("%s=%s", kv.key, kv.value)
	}
	workDir := "/work"
	c, err := r.client.ContainerCreate(ctx, &container.Config{
		Image: jr.actionJob.image,
		// Cmd:          []string{"/bin/sh", "-c", "apk update && apk add diffutils && cat LICENSE > package.json"}, // "timeout 12 while true; do echo 1; sleep 1; done"},
		Labels: map[string]string{"com.sourcegraph.runner": "true"},
		Env:    env,
		Tty:    false,
		// todo: only needed when piping in commands
		AttachStdin: true,
		OpenStdin:   true,
		StdinOnce:   true,
		// end todo
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
	jr.container = &c.ID

	hij, err := r.client.ContainerAttach(ctx, *jr.container, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		return err
	}
	defer hij.Close()

	err = r.client.ContainerStart(ctx, *jr.container, types.ContainerStartOptions{})
	if err != nil {
		return err
	}
	changeStatus(job, "RUNNING")
	attachCh := make(chan error, 2)
	shellOpts := "set -eo pipefail\n"
	go func() {
		_, err := io.Copy(hij.Conn, bytes.NewBufferString(shellOpts+"ls -laHN\ncat LICENSE >> package.json"))
		hij.CloseWrite()
		if err != nil {
			attachCh <- err
		}
	}()
	go func() {
		buffer := bytes.NewBuffer([]byte{})
		go func() {
			_, err := stdcopy.StdCopy(buffer, buffer, hij.Reader)
			if err != nil {
				println(err.Error())
				attachCh <- err
			}
		}()
		for {
			// todo: buffer might be written to during read, need concurrency lock
			if err := appendLog(job, buffer.String()); err != nil {
				attachCh <- err
				break
			}
			select {
			case <-ctx.Done():
				if err := appendLog(job, buffer.String()); err != nil {
					attachCh <- err
				}
				return
			case <-time.After(time.Second * 5):
			}
		}
	}()

	waitCh, errCh := r.client.ContainerWait(ctx, *jr.container, container.WaitConditionNotRunning)

	select {
	case <-ctx.Done():
		// e.killContainer(id, waitCh)
		return errors.New("Aborted")

	case err = <-attachCh:
		// e.killContainer(id, waitCh)
		if err != nil {
			return err
		}

	case res := <-waitCh:
		if res.StatusCode != 0 || res.Error != nil {
			// log job has failed
			fmt.Printf("Container failed with status code %d\n", res.StatusCode)
			if res.Error != nil {
				println(res.Error.Message)
			}
			changeStatus(job, "ERRORED")
			// return errors.New("Container errored")
		} else {
			changeStatus(job, "COMPLETED")
		}
	case err = <-errCh:
		if err != nil {
			return err
		}
	}
	// todo: error handling
	// Compute diff.
	oldDir, err := unzipToTempDir(ctx, zipFile.Name(), prefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(oldDir)

	diffOut, err := diffDirs(ctx, oldDir, volumeDir)
	if err != nil {
		return err // errors.Wrap(err, "Generating a diff failed")
	}

	// Strip temp dir prefixes from diff.
	fileDiffs, err := diff.ParseMultiFileDiff(diffOut)
	if err != nil {
		return err
	}
	for _, fileDiff := range fileDiffs {
		for i := range fileDiff.Extended {
			fileDiff.Extended[i] = strings.Replace(fileDiff.Extended[i], oldDir+string(os.PathSeparator), "", -1)
			fileDiff.Extended[i] = strings.Replace(fileDiff.Extended[i], volumeDir+string(os.PathSeparator), "", -1)
		}
		fileDiff.OrigName = strings.TrimPrefix(fileDiff.OrigName, oldDir+string(os.PathSeparator))
		fileDiff.NewName = strings.TrimPrefix(fileDiff.NewName, volumeDir+string(os.PathSeparator))
	}
	d, err := diff.PrintMultiFileDiff(fileDiffs)
	if err != nil {
		return err
	}
	parsedD := string(d)
	if parsedD != "" {
		job.diff = &parsedD
	}
	return nil
}

func (r *runner) checkForJobs(ctx context.Context) (*actionJob, error) {
	println("Checking for new jobs..")
	var result struct {
		BeginActionJob *struct {
			ID         string `json:"id"`
			Definition struct {
				Steps string `json:"steps"`
				Env   []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"env"`
				ActionWorkspace struct {
					Name string `json:"name"`
				} `json:"actionWorkspace"`
			} `json:"definition"`
			Repository struct {
				Name string `json:"name"`
			} `json:"repository"`
			BaseRevision string `json:"baseRevision"`
		} `json:"pullActionJob,omitempty"`
	}
	query := `mutation BeginActionJob($runner: ID!) {
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
		return nil, err
	}
	if result.BeginActionJob != nil {
		fmt.Printf("Got job with ID '%s'\n", result.BeginActionJob.ID)
		return &actionJob{ID: result.BeginActionJob.ID, image: result.BeginActionJob.Definition.Steps}, nil
	}
	return nil, nil
}

func appendLog(job *actionJob, content string) error {
	if content == "" {
		return nil
	}
	var result struct{}
	query := `mutation AppendLog($actionJob: ID!, $content: String!) {
	appendLog(actionJob: $actionJob, content: $content) {
		alwaysNil
	}
}`
	if err := (&apiRequest{
		query: query,
		vars: map[string]interface{}{
			"actionJob": "jobid",
			"content":   content,
		},
		result: &result,
	}).do(); err != nil {
		return err
	}
	return nil
}

func changeStatus(job *actionJob, status string) error {
	fmt.Printf("Status of container changed to %s\n", status)
	if status == "PULLING" || status == "PREPARING" || status == "CREATING" {
		return nil
	}
	var result struct{}
	query := `mutation UpdateActionJob($actionJob: ID!, $state: ActionJobState) {
	updateActionJob(actionJob: $actionJob, state: $state) {
		id
	}
}`
	if err := (&apiRequest{
		query: query,
		vars: map[string]interface{}{
			"actionJob": job.ID,
			"state":     status,
		},
		result: &result,
	}).do(); err != nil {
		println(err.Error())
		return err
	}
	return nil
}
