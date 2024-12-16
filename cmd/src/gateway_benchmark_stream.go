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
	url        string
	authHeader string
	body       string
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
    $ src gateway benchmark-stream --gateway http://localhost:9992 --sourcegraph http://localhost:3082 --sgd <token> --sgp <token> --max-tokens 50
`

	flagSet := flag.NewFlagSet("benchmark-stream", flag.ExitOnError)

	var (
		requestCount    = flagSet.Int("requests", 1000, "Number of requests to make per endpoint")
		csvOutput       = flagSet.String("csv", "", "Export results to CSV file (provide filename)")
		gatewayEndpoint = flagSet.String("gateway", "", "Cody Gateway endpoint")
		sgEndpoint      = flagSet.String("sourcegraph", "", "Sourcegraph endpoint")
		sgdToken        = flagSet.String("sgd", "", "Sourcegraph Dotcom user key for Cody Gateway")
		sgpToken        = flagSet.String("sgp", "", "Sourcegraph personal access token for the called instance")
	)

	handler := func(args []string) error {
		// Parse the flags.
		if err := flagSet.Parse(args); err != nil {
			return err
		}
		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}
		if *gatewayEndpoint != "" && *sgdToken == "" {
			return cmderrors.Usage("must specify --sgp <Sourcegraph personal access token>")
		}
		if *sgEndpoint != "" && *sgpToken == "" {
			return cmderrors.Usage("must specify --sgp <Sourcegraph personal access token>")
		}

		var httpClient = &http.Client{}
		var results []endpointResult

		// Do the benchmarking.
		fmt.Printf("Starting benchmark with %d requests per endpoint...\n", *requestCount)
		if *gatewayEndpoint != "" {
			fmt.Println("Benchmarking Cody Gateway instance:", *gatewayEndpoint)
			cgResults50 := benchmarkCodeCompletions("gateway-50", httpClient, buildGatewayHttpEndpoint(*gatewayEndpoint, *sgdToken, 50), *requestCount)
			cgResults200 := benchmarkCodeCompletions("gateway-200", httpClient, buildGatewayHttpEndpoint(*gatewayEndpoint, *sgdToken, 200), *requestCount)
			cgResults500 := benchmarkCodeCompletions("gateway-500", httpClient, buildGatewayHttpEndpoint(*gatewayEndpoint, *sgdToken, 500), *requestCount)
			results = append(results, cgResults50, cgResults200, cgResults500)
			fmt.Println()
		} else {
			fmt.Println("warning: not benchmarking Cody Gateway (-gateway endpoint not provided)")
		}
		if *sgEndpoint != "" {
			fmt.Println("Benchmarking Sourcegraph instance:", *sgEndpoint)
			sgResults50 := benchmarkCodeCompletions("sourcegraph-50", httpClient, buildSourcegraphHttpEndpoint(*sgEndpoint, *sgpToken, 50), *requestCount)
			sgResults200 := benchmarkCodeCompletions("sourcegraph-200", httpClient, buildSourcegraphHttpEndpoint(*sgEndpoint, *sgpToken, 200), *requestCount)
			sgResults500 := benchmarkCodeCompletions("sourcegraph-500", httpClient, buildSourcegraphHttpEndpoint(*sgEndpoint, *sgpToken, 500), *requestCount)
			results = append(results, sgResults50, sgResults200, sgResults500)
			fmt.Println()
		} else {
			fmt.Println("warning: not benchmarking Sourcegraph instance (-sourcegraph endpoint not provided)")
		}

		// Output the results.
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

func buildGatewayHttpEndpoint(gatewayEndpoint string, sgdToken string, maxTokens int) httpEndpoint {
	return httpEndpoint{
		url:        fmt.Sprint(gatewayEndpoint, "/v1/completions/anthropic-messages"),
		authHeader: fmt.Sprintf("Bearer %s", sgdToken),
		body: fmt.Sprintf(`{
    "model": "claude-3-haiku-20240307",
    "messages": [
        {"role": "user", "content": "def bubble_sort(arr):"},
        {"role": "assistant", "content": "Here is a bubble sort:"}
    ],
    "max_tokens": %d,
    "temperature": 0.0,
    "stream": true
}`, maxTokens),
	}
}

func buildSourcegraphHttpEndpoint(sgEndpoint string, sgpToken string, maxTokens int) httpEndpoint {
	return httpEndpoint{
		url:        fmt.Sprint(sgEndpoint, "/.api/completions/stream"),
		authHeader: fmt.Sprintf("token %s", sgpToken),
		body: fmt.Sprintf(`{
    "model": "anthropic::2023-06-01::claude-3-haiku",
    "messages": [
        {"speaker": "human", "text": "def bubble_sort(arr):"},
        {"speaker": "assistant", "text": "Here is a bubble sort:"}
    ],
    "maxTokensToSample": %d,
    "temperature": 0.0,
    "stream": true
}`, maxTokens),
	}
}

func benchmarkCodeCompletions(benchmarkName string, client *http.Client, endpoint httpEndpoint, requestCount int) endpointResult {
	durations := make([]time.Duration, 0, requestCount)

	for i := 0; i < requestCount; i++ {
		duration := benchmarkCodeCompletion(client, endpoint)
		if duration > 0 {
			durations = append(durations, duration)
		}
	}
	stats := calculateStats(durations)

	return toEndpointResult(benchmarkName, stats, len(durations))
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
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("non-200 response: %v - %s\n", resp.Status, body)
		return 0
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return 0
	}

	return time.Since(start)
}

func toEndpointResult(name string, stats Stats, requestCount int) endpointResult {
	return endpointResult{
		name:       name,
		avg:        stats.Avg,
		median:     stats.Median,
		p5:         stats.P5,
		p75:        stats.P75,
		p80:        stats.P80,
		p95:        stats.P95,
		successful: requestCount,
		total:      stats.Total,
	}
}
