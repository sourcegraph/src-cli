package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/kballard/go-shellquote"
	"github.com/mattn/go-isatty"
)

func init() {
	usage := `
Examples:

  Upload an LSIF dump:

    	$ src lsif upload -repo=FOO -commit=BAR -upload-token=BAZ -file=data.lsif

`

	flagSet := flag.NewFlagSet("upload", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src lsif %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		repoFlag        = flagSet.String("repo", "", `The name of the repository. (required)`)
		commitFlag      = flagSet.String("commit", "", `The 40-character hash of the commit. (required)`)
		fileFlag        = flagSet.String("file", "", `The path to the LSIF dump file. (required)`)
		uploadTokenFlag = flagSet.String("upload-token", "", `The LSIF upload token for the given repository. (required for Sourcegraph.com only)`)
		apiFlags        = newAPIFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		if *repoFlag == "" || *commitFlag == "" || *fileFlag == "" {
			usageFunc()
			os.Exit(2)
		}

		qs := url.Values{}
		qs.Add("repository", *repoFlag)
		qs.Add("commit", *commitFlag)
		if *uploadTokenFlag != "" {
			qs.Add("upload_token", *uploadTokenFlag)
		}

		url, err := url.Parse(cfg.Endpoint + "/.api/lsif/upload")
		if err != nil {
			return err
		}
		url.RawQuery = qs.Encode()

		// Handle the get-curl flag now.
		if *apiFlags.getCurl {
			curl := fmt.Sprintf("gzip %s | curl \\\n", shellquote.Join(*fileFlag))
			curl += fmt.Sprintf("   -X POST \\\n")
			if cfg.AccessToken != "" {
				curl += fmt.Sprintf("   %s \\\n", shellquote.Join("-H", "Authorization: token "+cfg.AccessToken))
			}

			curl += fmt.Sprintf("   %s \\\n", shellquote.Join("-H", "Content-Type: application/x-ndjson+lsif"))
			curl += fmt.Sprintf("   %s \\\n", shellquote.Join("", url.String()))
			curl += fmt.Sprintf("   %s", shellquote.Join("--data-binary", "@-"))

			fmt.Println(curl)
			return nil
		}

		f, err := os.Open(*fileFlag)
		if err != nil {
			return err
		}
		defer f.Close()

		g, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer g.Close()

		// Create the HTTP request.
		req, err := http.NewRequest("POST", url.String(), f)
		if err != nil {
			return err
		}

		if cfg.AccessToken != "" {
			req.Header.Set("Authorization", "token "+cfg.AccessToken)
		}

		req.Header.Set("Content-Type", "application/x-ndjson+lsif")

		// Perform the request.
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Our request may have failed before the reaching GraphQL endpoint, so
		// confirm the status code. You can test this easily with e.g. an invalid
		// endpoint like -endpoint=https://google.com
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusUnauthorized && isatty.IsCygwinTerminal(os.Stdout.Fd()) {
				fmt.Println("You may need to specify or update your access token to use this endpoint.")
				fmt.Println("See https://github.com/sourcegraph/src-cli#authentication")
				fmt.Println("")
			}
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("error: %s\n\n%s", resp.Status, body)
		}

		fmt.Printf("LSIF dump uploaded.\n")
		return nil
	}

	// Register the command.
	lsifCommands = append(lsifCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
