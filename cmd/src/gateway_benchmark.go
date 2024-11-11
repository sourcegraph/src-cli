package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const (
	defaultRequestCount = 100
	blue                = "\033[34m"
	green       = "\033[32m"
	yellow      = "\033[33m"
	red         = "\033[31m"
	reset       = "\033[0m"
)

func init() {
	usage := `
'src gateway benchmark' runs performance benchmarks against Cody Gateway endpoints.

Usage:

    src gateway benchmark [flags]

Examples:

    $ src gateway benchmark
    $ src gateway benchmark --requests 50
`

	flagSet := flag.NewFlagSet("benchmark", flag.ExitOnError)

	var (
		requestCount = flagSet.Int("requests", defaultRequestCount, "Number of requests to make per endpoint")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		// Create HTTP client with TLS skip verify
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		endpoints := map[string]string{
			"HTTP":                fmt.Sprintf("%s/cody-gateway-call-http", cfg.Endpoint),
			"HTTP then WebSocket": fmt.Sprintf("%s/cody-gateway-call-http-then-websocket", cfg.Endpoint),
		}

		fmt.Printf("Starting benchmark with %d requests per endpoint...\n", *requestCount)

		var results []endpointResult

		for name, url := range endpoints {
			durations := make([]time.Duration, 0, *requestCount)
			fmt.Printf("\nTesting %s...", name)

			for i := 0; i < *requestCount; i++ {
				duration := benchmarkEndpoint(client, url)
				if duration > 0 {
					durations = append(durations, duration)
				}
				fmt.Printf("\rTesting %s: %d/%d", name, i+1, *requestCount)
			}
			fmt.Println()

			avg, p5, p95, median, total := calculateStats(durations)

			results = append(results, endpointResult{
				name:       name,
				avg:        avg,
				median:     median,
				p5:         p5,
				p95:        p95,
				total:      total,
				successful: len(durations),
			})
		}

		printResults(results, requestCount)
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

type endpointResult struct {
	name       string
	avg        time.Duration
	median     time.Duration
	p5         time.Duration
	p95        time.Duration
	total      time.Duration
	successful int
}

func benchmarkEndpoint(client *http.Client, url string) time.Duration {
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Error calling %s: %v\n", url, err)
		return 0
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return 0
	}
	fmt.Printf("Response from %s: %s\n", url, body)

	return time.Since(start)
}

func calculateStats(durations []time.Duration) (time.Duration, time.Duration, time.Duration, time.Duration, time.Duration) {
	if len(durations) == 0 {
		return 0, 0, 0, 0, 0
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	avg := sum / time.Duration(len(durations))

	p5idx := int(float64(len(durations)) * 0.05)
	p95idx := int(float64(len(durations)) * 0.95)
	medianIdx := len(durations) / 2

	return avg, durations[p5idx], durations[p95idx], durations[medianIdx], sum
}

func formatDuration(d time.Duration, best bool, worst bool) string {
	value := fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
	if best {
		return green + value + reset
	}
	if worst {
		return red + value + reset
	}
	return yellow + value + reset
}

func formatSuccessRate(successful, total int, best bool, worst bool) string {
	value := fmt.Sprintf("%d/%d", successful, total)
	if best {
		return green + value + reset
	}
	if worst {
		return red + value + reset
	}
	return yellow + value + reset
}

func printResults(results []endpointResult, requestCount *int) {
	// Print header
	headerFmt := blue + "%-20s | %-10s | %-10s | %-10s | %-10s | %-10s | %-10s" + reset + "\n"
	fmt.Printf("\n"+headerFmt,
		"Endpoint    ", "Average", "Median", "P5", "P95", "Total", "Success")
	fmt.Println(blue + strings.Repeat("-", 96) + reset)

	// Find best/worst values for each metric
	var bestAvg, worstAvg time.Duration
	var bestMedian, worstMedian time.Duration
	var bestP5, worstP5 time.Duration
	var bestP95, worstP95 time.Duration
	var bestTotal, worstTotal time.Duration
	var bestSuccess, worstSuccess int

	for i, r := range results {
		if i == 0 || r.avg < bestAvg {
			bestAvg = r.avg
		}
		if i == 0 || r.avg > worstAvg {
			worstAvg = r.avg
		}
		if i == 0 || r.median < bestMedian {
			bestMedian = r.median
		}
		if i == 0 || r.median > worstMedian {
			worstMedian = r.median
		}
		if i == 0 || r.p5 < bestP5 {
			bestP5 = r.p5
		}
		if i == 0 || r.p5 > worstP5 {
			worstP5 = r.p5
		}
		if i == 0 || r.p95 < bestP95 {
			bestP95 = r.p95
		}
		if i == 0 || r.p95 > worstP95 {
			worstP95 = r.p95
		}
		if i == 0 || r.total < bestTotal {
			bestTotal = r.total
		}
		if i == 0 || r.total > worstTotal {
			worstTotal = r.total
		}
		if i == 0 || r.successful > bestSuccess {
			bestSuccess = r.successful
		}
		if i == 0 || r.successful < worstSuccess {
			worstSuccess = r.successful
		}
	}

	// Print each row
	for _, r := range results {
		fmt.Printf("%-20s | %-19s | %-19s | %-19s | %-19s | %-19s | %s\n",
			r.name,
			formatDuration(r.avg, r.avg == bestAvg, r.avg == worstAvg),
			formatDuration(r.median, r.median == bestMedian, r.median == worstMedian),
			formatDuration(r.p5, r.p5 == bestP5, r.p5 == worstP5),
			formatDuration(r.p95, r.p95 == bestP95, r.p95 == worstP95),
			formatDuration(r.total, r.total == bestTotal, r.total == worstTotal),
			formatSuccessRate(r.successful, *requestCount, r.successful == bestSuccess, r.successful == worstSuccess))
	}
}
