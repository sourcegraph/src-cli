package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

func init() {
	usage := `
Examples:

  Get the src-cli version and the Sourcegraph instance's version:

    	$ src version
`

	flagSet := flag.NewFlagSet("version", flag.ExitOnError)

	handler := func(args []string) error {
		version, err := getCurrentVersion()
		if err != nil {
			return err
		}
		fmt.Printf("Current version: %s\n", version)

		recommendedVersion, err := getRecommendedVersion()
		if err != nil {
			return err
		}
		if recommendedVersion == "" {
			fmt.Println("Recommended Version: <unknown>")
			fmt.Println("This Sourcegraph instance does not support this feature.")
			return nil
		}
		fmt.Printf("Recommended Version: %s\n", recommendedVersion)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func getCurrentVersion() (string, error) {
	// TODO
	return "dev", nil
}

func getRecommendedVersion() (string, error) {
	url, err := url.Parse(cfg.Endpoint + "/.api/src-cli/version")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("GET", url.String(), nil)
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
		if resp.StatusCode == http.StatusNotFound {
			return "", nil
		}

		return "", fmt.Errorf("error: %s\n\n%s", resp.Status, body)
	}

	payload := struct {
		Version string `json:"version"`
	}{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	return payload.Version, nil
}
