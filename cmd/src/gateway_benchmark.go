package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"

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
'src gateway benchmark' runs performance benchmarks against Cody Gateway and Sourcegraph test endpoints.

Usage:

    src gateway benchmark [flags]

Examples:

    $ src gateway benchmark --sgp <token>
    $ src gateway benchmark --requests 50 --sgp <token>
    $ src gateway benchmark --gateway http://localhost:9992 --sourcegraph http://localhost:3082 --sgp <token>
    $ src gateway benchmark --requests 50 --csv results.csv --sgp <token>
    $ src gateway benchmark --gateway https://cody-gateway.sourcegraph.com --sourcegraph https://sourcegraph.com --sgp <token> --use-special-header
`

	flagSet := flag.NewFlagSet("benchmark", flag.ExitOnError)

	var (
		requestCount    = flagSet.Int("requests", 1000, "Number of requests to make per endpoint")
		csvOutput       = flagSet.String("csv", "", "Export results to CSV file (provide filename)")
		gatewayEndpoint = flagSet.String("gateway", "https://cody-gateway.sourcegraph.com", "Cody Gateway endpoint")
		sgEndpoint      = flagSet.String("sourcegraph", "https://sourcegraph.com", "Sourcegraph endpoint")
		sgpToken        = flagSet.String("sgp", "", "Sourcegraph personal access token for the called instance")
		useSpecialHeader = flagSet.Bool("use-special-header", false, "Use special header to test the gateway")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		if *useSpecialHeader {
			fmt.Println("Using special header 'cody-core-gc-test'")
		}

		var (
			httpClient = &http.Client{}
			endpoints  = map[string]any{} // Values: URL `string`s or `*webSocketClient`s
		)
		if *gatewayEndpoint != "" {
			fmt.Println("Benchmarking Cody Gateway instance:", *gatewayEndpoint)
			endpoints["ws(s): gateway"] = &webSocketClient{
				conn: nil,
				URL:  strings.Replace(fmt.Sprint(*gatewayEndpoint, "/v2/websocket"), "http", "ws", 1),
			}
			endpoints["http(s): gateway"] = fmt.Sprint(*gatewayEndpoint, "/v2/http")
		} else {
			fmt.Println("warning: not benchmarking Cody Gateway (-gateway endpoint not provided)")
		}
		if *sgEndpoint != "" {
			if *sgpToken == "" {
				return cmderrors.Usage("must specify --sgp <Sourcegraph personal access token>")
			}
			fmt.Println("Benchmarking Sourcegraph instance:", *sgEndpoint)
			headers := http.Header{
				"Authorization": []string{"token " + *sgpToken},
			}
			if *useSpecialHeader {
				headers.Set("cody-core-gc-test", "M2R{+6VI?1,M3n&<vpw1&AK>")
			}

			endpoints["ws(s): sourcegraph"] = &webSocketClient{
				conn:    nil,
				URL:     strings.Replace(fmt.Sprint(*sgEndpoint, "/.api/gateway/websocket"), "http", "ws", 1),
				headers: headers,
			}
			endpoints["http(s): sourcegraph"] = fmt.Sprint(*sgEndpoint, "/.api/gateway/http")
			endpoints["http(s): http-then-ws"] = fmt.Sprint(*sgEndpoint, "/.api/gateway/http-then-websocket")
		} else {
			fmt.Println("warning: not benchmarking Sourcegraph instance (-sourcegraph endpoint not provided)")
		}

		fmt.Printf("Starting benchmark with %d requests per endpoint...\n", *requestCount)

		var results []endpointResult
		for name, clientOrURL := range endpoints {
			durations := make([]time.Duration, 0, *requestCount)
			fmt.Printf("\nTesting %s...", name)

			for i := 0; i < *requestCount; i++ {
				if ws, ok := clientOrURL.(*webSocketClient); ok {
					duration := benchmarkEndpointWebSocket(ws)
					if duration > 0 {
						durations = append(durations, duration)
					}
				} else if url, ok := clientOrURL.(string); ok {
					duration := benchmarkEndpointHTTP(httpClient, url, *sgpToken, *useSpecialHeader)
					if duration > 0 {
						durations = append(durations, duration)
					}
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

type webSocketClient struct {
	conn    *websocket.Conn
	URL     string
	headers http.Header
}

func (c *webSocketClient) reconnect() error {
	if c.conn != nil {
		c.conn.Close() // don't leak connections
	}
	fmt.Println("Connecting to WebSocket..", c.URL)
	var err error
	c.conn, _, err = websocket.DefaultDialer.Dial(c.URL, c.headers)
	if err != nil {
		c.conn = nil // retry again later
		return fmt.Errorf("WebSocket dial(%s): %v", c.URL, err)
	}
	fmt.Println("Connected!")
	return nil
}

type endpointResult struct {
	name       string
	avg        time.Duration
	median     time.Duration
	p5         time.Duration
	p75        time.Duration
	p80        time.Duration
	p95        time.Duration
	total      time.Duration
	successful int
}

func benchmarkEndpointHTTP(client *http.Client, url, accessToken string, useSpecialHeader bool) time.Duration {
	start := time.Now()
	req, err := http.NewRequest("POST", url, strings.NewReader("ping"))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+accessToken)
	if useSpecialHeader {
		req.Header.Set("cody-core-gc-test", "M2R{+6VI?1,M3n&<vpw1&AK>")
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error calling %s: %v\n", url, err)
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return 0
	}
	if string(body) != "pong" {
		fmt.Printf("Expected 'pong' response, got: %q\n", string(body))
		return 0
	}

	return time.Since(start)
}

func benchmarkEndpointWebSocket(client *webSocketClient) time.Duration {
	// Perform initial websocket connection, if needed.
	if client.conn == nil {
		if err := client.reconnect(); err != nil {
			fmt.Printf("Error reconnecting: %v\n", err)
			return 0
		}
	}

	// Perform the benchmarked request using the websocket.
	start := time.Now()
	err := client.conn.WriteMessage(websocket.TextMessage, []byte("ping"))
	if err != nil {
		fmt.Printf("WebSocket write error: %v\n", err)
		if err := client.reconnect(); err != nil {
			fmt.Printf("Error reconnecting: %v\n", err)
		}
		return 0
	}
	_, message, err := client.conn.ReadMessage()

	if err != nil {
		fmt.Printf("WebSocket read error: %v\n", err)
		if err := client.reconnect(); err != nil {
			fmt.Printf("Error reconnecting: %v\n", err)
		}
		return 0
	}
	if string(message) != "pong" {
		fmt.Printf("Expected 'pong' response, got: %q\n", string(message))
		if err := client.reconnect(); err != nil {
			fmt.Printf("Error reconnecting: %v\n", err)
		}
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
	headerFmt := ansiColors["blue"] + "%-25s | %-10s | %-10s | %-10s | %-10s | %-10s | %-10s | %-10s | %-10s" + ansiColors["nc"] + "\n"
	fmt.Printf("\n"+headerFmt,
		"Endpoint    ", "Average", "Median", "P5", "P75", "P80", "P95", "Total", "Success")
	fmt.Println(ansiColors["blue"] + strings.Repeat("-", 121) + ansiColors["nc"])

	// Find best/worst values for each metric
	var bestAvg, worstAvg time.Duration
	var bestMedian, worstMedian time.Duration
	var bestP5, worstP5 time.Duration
	var bestP75, worstP75 time.Duration
	var bestP80, worstP80 time.Duration
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
		if i == 0 || r.p75 < bestP75 {
			bestP75 = r.p75
		}
		if i == 0 || r.p75 > worstP75 {
			worstP75 = r.p75
		}
		if i == 0 || r.p80 < bestP80 {
			bestP80 = r.p80
		}
		if i == 0 || r.p80 > worstP80 {
			worstP80 = r.p80
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
		fmt.Printf("%-25s | %-19s | %-19s | %-19s | %-19s | %-19s | %-19s | %-19s | %s\n",
			r.name,
			formatDuration(r.avg, r.avg == bestAvg, r.avg == worstAvg),
			formatDuration(r.median, r.median == bestMedian, r.median == worstMedian),
			formatDuration(r.p5, r.p5 == bestP5, r.p5 == worstP5),
			formatDuration(r.p75, r.p75 == bestP75, r.p75 == worstP75),
			formatDuration(r.p80, r.p80 == bestP80, r.p80 == worstP80),
			formatDuration(r.p95, r.p95 == bestP95, r.p95 == worstP95),
			formatDuration(r.total, r.total == bestTotal, r.total == worstTotal),
			formatSuccessRate(r.successful, *requestCount, r.successful == bestSuccess, r.successful == worstSuccess))
	}
}

func writeResultsToCSV(filename string, results []endpointResult, requestCount *int) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %v", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			return
		}
	}()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Endpoint", "Average (ms)", "Median (ms)", "P5 (ms)", "P75 (ms)", "P80 (ms)", "P95 (ms)", "Total (ms)", "Success Rate"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %v", err)
	}

	// Write data rows
	for _, r := range results {
		row := []string{
			r.name,
			fmt.Sprintf("%.2f", float64(r.avg.Microseconds())/1000),
			fmt.Sprintf("%.2f", float64(r.median.Microseconds())/1000),
			fmt.Sprintf("%.2f", float64(r.p5.Microseconds())/1000),
			fmt.Sprintf("%.2f", float64(r.p75.Microseconds())/1000),
			fmt.Sprintf("%.2f", float64(r.p80.Microseconds())/1000),
			fmt.Sprintf("%.2f", float64(r.p95.Microseconds())/1000),
			fmt.Sprintf("%.2f", float64(r.total.Microseconds())/1000),
			fmt.Sprintf("%d/%d", r.successful, *requestCount),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %v", err)
		}
	}

	return nil
}
