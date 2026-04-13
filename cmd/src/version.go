package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/version"

	"github.com/urfave/cli/v3"
)

const versionExamples = `Examples:

  Get the src-cli version and the Sourcegraph instance's recommended version:

    	$ src version
`

var versionCommandv2 = clicompat.Wrap(&cli.Command{
	Name:         "version",
	Usage:        "display and compare the src-cli version against the recommended version for your instance",
	UsageText:    "src version [options]",
	OnUsageError: clicompat.OnUsageError,
	Description: `
` + versionExamples,
	Flags: clicompat.WithAPIFlags(
		&cli.BoolFlag{
			Name:  "client-only",
			Usage: "If true, only the client version will be printed.",
		},
	),
	HideVersion: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		args := VersionArgs{
			Client:     cfg.apiClient(clicompat.APIFlagsFromCmd(c), os.Stdout),
			ClientOnly: c.Bool("client-only"),
		}
		return versionHandler(args)
	},
})

type VersionArgs struct {
	ClientOnly bool
	Client     api.Client
	Output     io.Writer
}

func versionHandler(args VersionArgs) error {
	fmt.Printf("Current version: %s\n", version.BuildTag)
	if args.ClientOnly {
		return nil
	}

	recommendedVersion, err := getRecommendedVersion(context.Background(), args.Client)
	if err != nil {
		return errors.Wrap(err, "failed to get recommended version for Sourcegraph deployment")
	}
	if recommendedVersion == "" {
		fmt.Println("Recommended version: <unknown>")
		fmt.Println("This Sourcegraph instance does not support this feature.")
		return nil
	}
	fmt.Printf("Recommended version: %s or later\n", recommendedVersion)
	return nil
}

func getRecommendedVersion(ctx context.Context, client api.Client) (string, error) {
	req, err := client.NewHTTPRequest(ctx, "GET", ".api/src-cli/version", nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
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
