package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

type httpEndpoint struct {
	url string
	authHeader string
	body string
}

func init() {
	usage := `
'src gateway benchmark-stream' runs performance benchmarks against Cody Gateway and Sourcegraph
code completion streaming endpoints.

Usage:

    src gateway benchmark-stream [flags]

Examples:

    $ src gateway benchmark-stream --requests 50 --csv results.csv --sgd <token> --sgp <token>
    $ src gateway benchmark-stream --gateway http://localhost:9992 --sourcegraph http://localhost:3082 --sgd <token> --sgp <token>
`

	flagSet := flag.NewFlagSet("benchmark-stream", flag.ExitOnError)

	var (
		requestCount    = flagSet.Int("requests", 1000, "Number of requests to make per endpoint")
		csvOutput       = flagSet.String("csv", "", "Export results to CSV file (provide filename)")
		gatewayEndpoint = flagSet.String("gateway", "https://cody-gateway.sourcegraph.com", "Cody Gateway endpoint")
		sgEndpoint      = flagSet.String("sourcegraph", "https://sourcegraph.com", "Sourcegraph endpoint")
		sgdToken        = flagSet.String("sgd", "", "Sourcegraph Dotcom user key for Cody Gateway")
		sgpToken        = flagSet.String("sgp", "", "Sourcegraph personal access token for the called instance")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		var (
			httpClient = &http.Client{}
			endpoints  = map[string]httpEndpoint{}
		)
		if *gatewayEndpoint != "" {
			if *sgdToken == "" {
				return cmderrors.Usage("must specify --sgp <Sourcegraph personal access token>")
			}
			fmt.Println("Benchmarking Cody Gateway instance:", *gatewayEndpoint)
			endpoints["gateway"] = httpEndpoint{
				url: fmt.Sprint(*gatewayEndpoint, "/v1/completions/anthropic-messages"),
				authHeader: fmt.Sprintf("Bearer %s", *sgdToken),
				body: `{
    "model": "claude-3-haiku-20240307",
    "messages": [
        {"role": "user", "content": "def bubble_sort(arr):"},
        {"role": "assistant", "content": "Here is a bubble sort:"}
    ],
    "n": 1,
    "max_tokens": 200,
    "temperature": 0.0,
    "top_p": 0.95,
    "stream": true
}`,
			}
		} else {
			fmt.Println("warning: not benchmarking Cody Gateway (-gateway endpoint not provided)")
		}
		if *sgEndpoint != "" {
			if *sgpToken == "" {
				return cmderrors.Usage("must specify --sgp <Sourcegraph personal access token>")
			}
			fmt.Println("Benchmarking Sourcegraph instance:", *sgEndpoint)
			endpoints["sourcegraph"] = httpEndpoint{
				url: fmt.Sprint(*sgEndpoint, "/.api/completions/stream"),
				authHeader: fmt.Sprintf("token %s", *sgpToken),
				body: `{
    "model": "anthropic::2023-06-01::claude-3-haiku", 

    "messages": [
        {
            "speaker": "human",
            "text": "def bubble_sort(arr):"
        },
        {
            "speaker": "assistant",
            "text": "Here is a bubble sort:"
        }
    ],
    "maxTokensToSample": 200,
    "stream": true
}`,
			}
		} else {
			fmt.Println("warning: not benchmarking Sourcegraph instance (-sourcegraph endpoint not provided)")
		}

		fmt.Printf("Starting benchmark with %d requests per endpoint...\n", *requestCount)

		var results []endpointResult
		for name, endpoint := range endpoints {
			durations := make([]time.Duration, 0, *requestCount)
			fmt.Printf("\nTesting %s...", name)

			for i := 0; i < *requestCount; i++ {
				duration := benchmarkCodeCompletion(httpClient, endpoint)
				if duration > 0 {
					durations = append(durations, duration)
				}
			}
			fmt.Println()

			stats := calculateStats(durations)

			results = append(results, endpointResult{
				name:       name,
				avg:        stats.Avg,
				median:     stats.Median,
				p5:         stats.P5,
				p75:        stats.P75,
				p80:        stats.P80,
				p95:        stats.P95,
				total:      stats.Total,
				successful: len(durations),
			})
		}

		printResults(results, requestCount)

		if *csvOutput != "" {
			if err := writeResultsToCSV(*csvOutput, results, requestCount); err != nil {
				return fmt.Errorf("failed to export CSV: %v", err)
			}
			fmt.Printf("\nResults exported to %s\n", *csvOutput)
		}

		return nil
	}

	gatewayCommands = append(gatewayCommands, &command{
		flagSet: flagSet,
		aliases: []string{},
		handler: handler,
		usageFunc: func() {
			_, err := fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src gateway %s':\n", flagSet.Name())
			if err != nil {
				return
			}
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func benchmarkCodeCompletion(client *http.Client, endpoint httpEndpoint) time.Duration {
	start := time.Now()
	req, err := http.NewRequest("POST", endpoint.url, strings.NewReader(endpoint.body))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", endpoint.authHeader)
	req.Header.Set("X-Sourcegraph-Should-Trace", "true")
	req.Header.Set("X-Sourcegraph-Feature", "code_completions")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error calling %s: %v\n", endpoint.url, err)
		return 0
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("non-200 response: %v\n", resp.Status)
		return 0
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return 0
	}

	return time.Since(start)
}
