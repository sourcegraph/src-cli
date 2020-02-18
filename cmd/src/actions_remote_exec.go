package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gosuri/uiprogress"
	"github.com/pkg/errors"
)

func init() {
	usage := `
Sends an action definition to a src actions agent listening on -addr

TODO
`

	flagSet := flag.NewFlagSet("remote-exec", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src actions %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		addrFlag = flagSet.String("addr", "http://localhost:8080", "The address of the agent.")
		fileFlag = flagSet.String("f", "-", "The action file. If not given or '-' standard input is used. (Required)")
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		var (
			actionFile []byte
			err        error
		)

		if *fileFlag == "-" {
			actionFile, err = ioutil.ReadAll(os.Stdin)
		} else {
			actionFile, err = ioutil.ReadFile(*fileFlag)
		}
		if err != nil {
			return err
		}

		id, err := startRemoteExecution(*addrFlag, bytes.NewReader(actionFile))
		if err != nil {
			return err
		}

		if id == "" {
			return errors.New("Agent did not return ID for execution")
		}

		started := make(map[string]struct{})
		finished := make(map[string]struct{})

		var (
			progress *remoteExecProgressResponse
			ui       *uiprogress.Progress
			bar      *uiprogress.Bar
		)
		for progress == nil || !progress.Done {
			progress, err = getExecutionProgress(*addrFlag, id)
			if err != nil {
				return err
			}

			if bar == nil {
				ui = uiprogress.New()
				ui.Start()
				ui.SetOut(os.Stderr)
				go ui.Listen()

				bar = ui.AddBar(len(progress.Repos) * 2) // twice, because we increment for start + finish
				bar.PrependElapsed()
				bar.PrependFunc(func(b *uiprogress.Bar) string {
					return fmt.Sprintf("%d repositories (%d started, %d finished)", len(progress.Repos), len(started), len(finished))
				})
			}

			for _, r := range progress.Repos {
				repo := r.Repo
				status := r.Status

				patchDone := status.Patch != CampaignPlanPatch{}

				repoStarted := !status.StartedAt.IsZero()
				if _, ok := started[repo.Name]; !ok && (repoStarted || patchDone) {
					bar.Incr()
					started[repo.Name] = struct{}{}
				}

				repoFinished := !status.FinishedAt.IsZero()
				if _, ok := finished[repo.Name]; !ok && (repoFinished || patchDone) {
					bar.Incr()
					finished[repo.Name] = struct{}{}
				}
			}

			if !progress.Done {
				time.Sleep(1 * time.Second)
			}
		}

		return json.NewEncoder(os.Stdout).Encode(progress.Patches)
	}

	// Register the command.
	actionsCommands = append(actionsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func startRemoteExecution(addr string, action io.Reader) (string, error) {
	url, err := url.Parse(addr + "/exec")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url.String(), action)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error: %s\n\n%s", resp.Status, body)
	}

	var payload remoteExecResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	if payload.ID == "" {
		return "", errors.New("Agent did not return ID for execution")
	}

	return payload.ID, nil
}

func getExecutionProgress(addr, id string) (*remoteExecProgressResponse, error) {
	url, err := url.Parse(addr + "/progress/" + id)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error: %s\n\n%s", resp.Status, body)
	}

	var payload remoteExecProgressResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.Wrapf(err, "Failed to unmarshal progress response")
	}

	return &payload, nil
}
