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
    $ src gateway benchmark-stream --requests 250 --gateway http://localhost:9992 --sourcegraph http://localhost:3082 --sgd <token> --sgp <token> --max-tokens 50 --provider fireworks --stream
`

	flagSet := flag.NewFlagSet("benchmark-stream", flag.ExitOnError)

	var (
		requestCount          = flagSet.Int("requests", 1000, "Number of requests to make per endpoint")
		csvOutput             = flagSet.String("csv", "", "Export results to CSV file (provide filename)")
		requestLevelCsvOutput = flagSet.String("request-csv", "", "Export request results to CSV file (provide filename)")
		gatewayEndpoint       = flagSet.String("gateway", "", "Cody Gateway endpoint")
		sgEndpoint            = flagSet.String("sourcegraph", "", "Sourcegraph endpoint")
		sgdToken              = flagSet.String("sgd", "", "Sourcegraph Dotcom user key for Cody Gateway")
		sgpToken              = flagSet.String("sgp", "", "Sourcegraph personal access token for the called instance")
		maxTokens             = flagSet.Int("max-tokens", 256, "Maximum number of tokens to generate")
		provider              = flagSet.String("provider", "anthropic", "Provider to use for completion. Supported values: 'anthropic', 'fireworks'")
		stream                = flagSet.Bool("stream", false, "Whether to stream completions. Default: false")
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
		var cgResult, sgResult endpointResult
		var cgRequestResults, sgRequestResults []requestResult

		// Do the benchmarking.
		fmt.Printf("Starting benchmark with %d requests per endpoint...\n", *requestCount)
		if *gatewayEndpoint != "" {
			fmt.Println("Benchmarking Cody Gateway instance:", *gatewayEndpoint)
			endpoint := buildGatewayHttpEndpoint(*gatewayEndpoint, *sgdToken, *maxTokens, *provider, *stream)
			cgResult, cgRequestResults = benchmarkCodeCompletions("gateway", httpClient, endpoint, *requestCount)
			fmt.Println()
		} else {
			fmt.Println("warning: not benchmarking Cody Gateway (-gateway endpoint not provided)")
		}
		if *sgEndpoint != "" {
			fmt.Println("Benchmarking Sourcegraph instance:", *sgEndpoint)
			endpoint := buildSourcegraphHttpEndpoint(*sgEndpoint, *sgpToken, *maxTokens, *provider, *stream)
			sgResult, sgRequestResults = benchmarkCodeCompletions("sourcegraph", httpClient, endpoint, *requestCount)
			fmt.Println()
		} else {
			fmt.Println("warning: not benchmarking Sourcegraph instance (-sourcegraph endpoint not provided)")
		}

		// Output the results.
		endpointResults := []endpointResult{cgResult, sgResult}
		printResults(endpointResults, requestCount)
		if *csvOutput != "" {
			if err := writeResultsToCSV(*csvOutput, endpointResults, requestCount); err != nil {
				return fmt.Errorf("failed to export CSV: %v", err)
			}
			fmt.Printf("\nAggregate results exported to %s\n", *csvOutput)
		}
		if *requestLevelCsvOutput != "" {
			if err := writeRequestResultsToCSV(*requestLevelCsvOutput, map[string][]requestResult{"gateway": cgRequestResults, "sourcegraph": sgRequestResults}); err != nil {
				return fmt.Errorf("failed to export CSV: %v", err)
			}
			fmt.Printf("\nRequest-level results exported to %s\n", *requestLevelCsvOutput)
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

func buildGatewayHttpEndpoint(gatewayEndpoint string, sgdToken string, maxTokens int, provider string, stream bool) httpEndpoint {
	s := "true"
	if !stream {
		s = "false"
	}
	if provider == "anthropic" {
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
    "stream": %s
}`, maxTokens, s),
		}
	} else if provider == "fireworks" {
		return httpEndpoint{
			url:        fmt.Sprint(gatewayEndpoint, "/v1/completions/fireworks"),
			authHeader: fmt.Sprintf("Bearer %s", sgdToken),
			body: fmt.Sprintf(`{
    "model": "starcoder",
    "prompt": "#hello.ts<｜fim▁begin｜>const sayHello = () => <｜fim▁hole｜><｜fim▁end｜>",
    "max_tokens": %d,
    "stop": [
        "\n\n",
        "\n\r\n",
        "<｜fim▁begin｜>",
        "<｜fim▁hole｜>",
        "<｜fim▁end｜>, <|eos_token|>"
    ],
    "temperature": 0.2,
    "topK": 0,
    "topP": 0,
    "stream": %s
}`, maxTokens, s),
		}
	}

	return httpEndpoint{}
}

func buildSourcegraphHttpEndpoint(sgEndpoint string, sgpToken string, maxTokens int, provider string, stream bool) httpEndpoint {
	s := "true"
	if !stream {
		s = "false"
	}
	if provider == "anthropic" {
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
    "stream": %s
}`, maxTokens, s),
		}
	} else if provider == "fireworks" {
		return httpEndpoint{
			url:        fmt.Sprint(sgEndpoint, "/.api/completions/code"),
			authHeader: fmt.Sprintf("token %s", sgpToken),
			body: fmt.Sprintf(`{
    "model": "fireworks::v1::starcoder",
    "messages": [
        {"speaker": "human", "text": "#hello.ts<｜fim▁begin｜>const sayHello = () => <｜fim▁hole｜><｜fim▁end｜>"}
    ],
    "maxTokensToSample": %d,
    "stopSequences": [
        "\n\n",
        "\n\r\n",
        "<｜fim▁begin｜>",
        "<｜fim▁hole｜>",
        "<｜fim▁end｜>, <|eos_token|>"
    ],
    "temperature": 0.2,
    "topK": 0,
    "topP": 0,
    "stream": %s
}`, maxTokens, s),
		}
	}

	return httpEndpoint{}
}

func benchmarkCodeCompletions(benchmarkName string, client *http.Client, endpoint httpEndpoint, requestCount int) (endpointResult, []requestResult) {
	results := make([]requestResult, 0, requestCount)
	durations := make([]time.Duration, 0, requestCount)

	for i := 0; i < requestCount; i++ {
		result := benchmarkCodeCompletion(client, endpoint)
		if result.duration > 0 {
			results = append(results, result)
			durations = append(durations, result.duration)
		}
	}
	stats := calculateStats(durations)

	return toEndpointResult(benchmarkName, stats, len(durations)), results
}

func benchmarkCodeCompletion(client *http.Client, endpoint httpEndpoint) requestResult {
	start := time.Now()
	req, err := http.NewRequest("POST", endpoint.url, strings.NewReader(endpoint.body))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return requestResult{0, ""}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", endpoint.authHeader)
	req.Header.Set("X-Sourcegraph-Should-Trace", "true")
	req.Header.Set("X-Sourcegraph-Feature", "code_completions")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error calling %s: %v\n", endpoint.url, err)
		return requestResult{0, ""}
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
		return requestResult{0, ""}
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return requestResult{0, ""}
	}

	return requestResult{
		duration: time.Since(start),
		traceID:  resp.Header.Get("X-Trace"),
	}
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
