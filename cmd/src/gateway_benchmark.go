package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

type Stats struct {
	Avg    time.Duration
	P5     time.Duration
	P75    time.Duration
	P80    time.Duration
	P95    time.Duration
	Median time.Duration
	Total  time.Duration
}

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
		requestCount = flagSet.Int("requests", 1000, "Number of requests to make per endpoint")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		// Create HTTP client with TLS skip verify
		client := &http.Client{Transport: &http.Transport{}}

		endpoints := map[string]string{
			"HTTP":                fmt.Sprintf("%s/gateway", cfg.Endpoint),
			"HTTP then WebSocket": fmt.Sprintf("%s/gateway/http-then-websocket", cfg.Endpoint),
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
			}
			fmt.Println()

			stats := calculateStats(durations)

			results = append(results, endpointResult{
				name:       name,
				avg:        stats.Avg,
				median:     stats.Median,
				p5:         stats.P5,
				p95:        stats.P95,
				total:      stats.Total,
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

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return 0
	}

	return time.Since(start)
}

func calculateStats(durations []time.Duration) Stats {
	if len(durations) == 0 {
		return Stats{0, 0, 0, 0, 0, 0, 0}
	}

	// Sort durations in ascending order
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	avg := sum / time.Duration(len(durations))

	return Stats{
		Avg:    avg,
		P5:     durations[int(float64(len(durations))*0.05)],
		P75:    durations[int(float64(len(durations))*0.75)],
		P80:    durations[int(float64(len(durations))*0.80)],
		P95:    durations[int(float64(len(durations))*0.95)],
		Median: durations[(len(durations) / 2)],
		Total:  sum,
	}
}

func formatDuration(d time.Duration, best bool, worst bool) string {
	value := fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
	if best {
		return ansiColors["green"] + value + ansiColors["nc"]
	}
	if worst {
		return ansiColors["red"] + value + ansiColors["nc"]
	}
	return ansiColors["yellow"] + value + ansiColors["nc"]
}

func formatSuccessRate(successful, total int, best bool, worst bool) string {
	value := fmt.Sprintf("%d/%d", successful, total)
	if best {
		return ansiColors["green"] + value + ansiColors["nc"]
	}
	if worst {
		return ansiColors["red"] + value + ansiColors["nc"]
	}
	return ansiColors["yellow"] + value + ansiColors["nc"]
}

func printResults(results []endpointResult, requestCount *int) {
	// Print header
	headerFmt := ansiColors["blue"] + "%-20s | %-10s | %-10s | %-10s | %-10s | %-10s | %-10s" + ansiColors["nc"] + "\n"
	fmt.Printf("\n"+headerFmt,
		"Endpoint    ", "Average", "Median", "P5", "P95", "Total", "Success")
	fmt.Println(ansiColors["blue"] + strings.Repeat("-", 96) + ansiColors["nc"])

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
