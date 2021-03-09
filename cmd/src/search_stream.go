package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/streaming"
)

var labelRegexp *regexp.Regexp

func init() {
	labelRegexp, _ = regexp.Compile("(?:\\[)(.*?)(?:])")
}

// streamHandler handles search requests which contain the flag "stream".
// Requests are sent to search/stream instead of the GraphQL api.
func streamHandler(args []string) error {
	flagSet := flag.NewFlagSet("streaming search", flag.ExitOnError)
	var (
		display  = flagSet.Int("display", -1, "Limit the number of results that are displayed. Note that the statistics continue to report all results.")
		apiFlags = api.StreamingFlags(flagSet)
	)
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	query := flagSet.Arg(0)

	t, err := parseTemplate(streamingTemplate)
	if err != nil {
		panic(err)
	}

	// Create request.
	client := cfg.apiClient(apiFlags, flagSet.Output())
	req, err := client.NewHTTPRequest(context.Background(), "GET", "search/stream?q="+url.QueryEscape(query), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if *display >= 0 {
		q := req.URL.Query()
		q.Add("display", strconv.Itoa(*display))
		req.URL.RawQuery = q.Encode()
	}

	// Send request.
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Process response.
	err = streaming.Decoder{
		OnProgress: func(progress *streaming.Progress) {
			// We only show the final progress.
			if !progress.Done {
				return
			}
			err = t.ExecuteTemplate(os.Stdout, "progress", progress)
			if err != nil {
				_, _ = flagSet.Output().Write([]byte(fmt.Sprintf("error when executing template: %s\n", err)))
			}
			return
		},
		OnError: func(eventError *streaming.EventError) {
			fmt.Printf("ERR: %s", eventError.Message)
		},
		OnAlert: func(alert *streaming.EventAlert) {
			proposedQueries := make([]ProposedQuery, len(alert.ProposedQueries))
			for _, pq := range alert.ProposedQueries {
				proposedQueries = append(proposedQueries, ProposedQuery{
					Description: pq.Description,
					Query:       pq.Query,
				})
			}

			err = t.ExecuteTemplate(os.Stdout, "alert", searchResultsAlert{
				Title:           alert.Title,
				Description:     alert.Description,
				ProposedQueries: proposedQueries,
			})
			if err != nil {
				_, _ = flagSet.Output().Write([]byte(fmt.Sprintf("error when executing template: %s\n", err)))
				return
			}
		},
		OnMatches: func(matches []streaming.EventMatch) {
			for _, match := range matches {
				switch match := match.(type) {
				case *streaming.EventFileMatch:
					err = t.ExecuteTemplate(os.Stdout, "file", struct {
						Query string
						*streaming.EventFileMatch
					}{
						Query:          query,
						EventFileMatch: match,
					},
					)
					if err != nil {
						_, _ = flagSet.Output().Write([]byte(fmt.Sprintf("error when executing template: %s\n", err)))
						return
					}
				case *streaming.EventRepoMatch:
					err = t.ExecuteTemplate(os.Stdout, "repo", struct {
						SourcegraphEndpoint string
						*streaming.EventRepoMatch
					}{
						SourcegraphEndpoint: cfg.Endpoint,
						EventRepoMatch:      match,
					})
					if err != nil {
						_, _ = flagSet.Output().Write([]byte(fmt.Sprintf("error when executing template: %s\n", err)))
						return
					}
				case *streaming.EventCommitMatch:
					err = t.ExecuteTemplate(os.Stdout, "commit", struct {
						SourcegraphEndpoint string
						*streaming.EventCommitMatch
					}{
						SourcegraphEndpoint: cfg.Endpoint,
						EventCommitMatch:    match,
					})
					if err != nil {
						_, _ = flagSet.Output().Write([]byte(fmt.Sprintf("error when executing template: %s\n", err)))
						return
					}
				case *streaming.EventSymbolMatch:
					err = t.ExecuteTemplate(os.Stdout, "symbol", struct {
						SourcegraphEndpoint string
						*streaming.EventSymbolMatch
					}{
						SourcegraphEndpoint: cfg.Endpoint,
						EventSymbolMatch:    match,
					},
					)
					if err != nil {
						_, _ = flagSet.Output().Write([]byte(fmt.Sprintf("error when executing template: %s\n", err)))
						return
					}
				}
			}
		},
	}.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error during decoding: %w", err)
	}

	// Write trace to output.
	if apiFlags.Trace() {
		_, err := flagSet.Output().Write([]byte(fmt.Sprintf("x-trace: %s\n", resp.Header.Get("x-trace"))))
		if err != nil {
			return err
		}
	}

	return nil
}

const streamingTemplate = `
{{define "file"}}
	{{- /* Repository and file name */ -}}
	{{- color "search-repository"}}{{.Repository}}{{color "nc" -}}
	{{- " › " -}}
	{{- color "search-filename"}}{{.Path}}{{color "nc" -}}
	{{- color "success"}}{{matchOrMatches (len .LineMatches)}}{{color "nc" -}}
	{{- "\n" -}}
	{{- color "search-border"}}{{"--------------------------------------------------------------------------------\n"}}{{color "nc"}}
	
	{{- /* Line matches */ -}}
	{{- $lineMatches := .LineMatches -}}
	{{- range $index, $match := $lineMatches -}}
		{{- if not (streamSearchSequentialLineNumber $lineMatches $index) -}}
			{{- color "search-border"}}{{"  ------------------------------------------------------------------------------\n"}}{{color "nc"}}
		{{- end -}}
		{{- "  "}}{{color "search-line-numbers"}}{{pad (addInt32 $match.LineNumber 1) 6 " "}}{{color "nc" -}}
		{{- color "search-border"}}{{" |  "}}{{color "nc"}}{{streamSearchHighlightMatch $.Query $match }}
	{{- end -}}
	{{- "\n" -}}
{{- end -}}

{{define "symbol"}}
	{{- /* Repository and file name */ -}}
	{{- color "search-repository"}}{{.Repository}}{{color "nc" -}}
	{{- " › " -}}
	{{- color "search-filename"}}{{.Path}}{{color "nc" -}}
	{{- color "success"}}{{matchOrMatches (len .Symbols)}}{{color "nc" -}}
	{{- "\n" -}}
	{{- color "search-border"}}{{"--------------------------------------------------------------------------------\n"}}{{color "nc"}}
	
	{{- /* Symbols */ -}}
	{{- $symbols := .Symbols -}}
	{{- range $index, $match := $symbols -}}
		{{- color "success"}}{{.Name}}{{color "nc" -}} ({{.Kind}}{{if .ContainerName}}{{printf ", %s" .ContainerName}}{{end}})
		{{- color "search-border"}}{{" ("}}{{color "nc" -}}
		{{- color "search-repository"}}{{$.SourcegraphEndpoint}}/{{$match.URL}}{{color "nc" -}}
		{{- color "search-border"}}{{")\n"}}{{color "nc" -}}
	{{- end -}}
	{{- "\n" -}}
{{- end -}}

{{define "repo"}}
	{{- /* Link to the result */ -}}
	{{- color "success"}}{{.Repository}}{{color "nc" -}}
	{{- color "search-border"}}{{" ("}}{{color "nc" -}}
	{{- color "search-repository"}}{{$.SourcegraphEndpoint}}/{{.Repository}}{{color "nc" -}}
	{{- color "search-border"}}{{")"}}{{color "nc" -}}
	{{- color "success"}}{{" ("}}{{"1 match)"}}{{color "nc" -}}
	{{- "\n" -}}
{{- end -}}

{{define "commit"}}
	{{- /* Link to the result */ -}}
	{{- color "search-border"}}{{"("}}{{color "nc" -}}
	{{- color "search-link"}}{{$.SourcegraphEndpoint}}{{.URL}}{{color "nc" -}}
	{{- color "search-border"}}{{")\n"}}{{color "nc" -}}
	{{- color "nc" -}}
	
	{{- /* Repository > author name "commit subject" (time ago) */ -}}
	{{- color "search-commit-subject"}}{{(streamSearchRenderCommitLabel .Label)}}{{color "nc" -}}
	{{- color "success"}}{{matchOrMatches (len .Ranges)}}{{color "nc" -}}
	{{- "\n" -}}
	{{- color "search-border"}}{{"--------------------------------------------------------------------------------\n"}}{{color "nc"}}
	{{- color "search-border"}}{{color "nc"}}{{indent (streamSearchHighlightCommit .Content .Ranges) "  "}}
{{end}}

{{define "alert"}}
	{{- searchAlertRender . -}}
{{end}}

{{define "progress"}}
	{{- color "logo" -}}✱{{- color "nc" -}}
	{{- " " -}}
	{{- if eq .MatchCount 0 -}}
		{{- color "warning" -}}
	{{- else -}}
		{{- color "success" -}}
	{{- end -}}
	{{- .MatchCount -}}{{if len .Skipped}}+{{end}} results{{- color "nc" -}}
	{{- " in " -}}{{color "success"}}{{msDuration .DurationMs}}{{if .RepositoriesCount}}{{- color "nc" -}}
	{{- " from " -}}{{color "success"}}{{.RepositoriesCount}}{{- " Repositories" -}}{{- color "nc" -}}{{end}}
	{{- "\n" -}}
	{{if len .Skipped}}
		{{- "\n" -}}
		{{- "Some results excluded:" -}}
		{{- "\n" -}}
		{{- range $index, $skipped := $.Skipped -}}
			{{indent $skipped.Title "    "}}{{- "\n" -}}
		{{- end -}}
		{{- "\n" -}}
	{{- end -}}
{{- end -}}
`

var streamSearchTemplateFuncs = map[string]interface{}{
	"streamSearchHighlightMatch": func(query string, match streaming.EventLineMatch) string {
		var highlights []highlight
		if strings.Contains(query, "patterntype:structural") {
			highlights = streamConvertMatchToHighlights(match, false)
			return applyHighlightsForFile(match.Line, highlights)
		}

		highlights = streamConvertMatchToHighlights(match, true)
		return applyHighlights(match.Line, highlights, ansiColors["search-match"], ansiColors["nc"])
	},

	"streamSearchSequentialLineNumber": func(lineMatches []streaming.EventLineMatch, index int) bool {
		prevIndex := index - 1
		if prevIndex < 0 {
			return true
		}
		prevLineNumber := lineMatches[prevIndex].LineNumber
		lineNumber := lineMatches[index].LineNumber
		return prevLineNumber == lineNumber-1
	},

	"streamSearchHighlightCommit": func(content string, ranges [][3]int32) string {
		highlights := make([]highlight, len(ranges))
		for _, r := range ranges {
			highlights = append(highlights, highlight{
				line:      int(r[0]),
				character: int(r[1]),
				length:    int(r[2]),
			})
		}
		if strings.HasPrefix(content, "```diff") {
			return streamSearchHighlightDiffPreview(content, highlights)
		}
		return applyHighlights(stripMarkdownMarkers(content), highlights, ansiColors["search-match"], ansiColors["nc"])
	},

	"streamSearchRenderCommitLabel": func(label string) string {
		m := labelRegexp.FindAllStringSubmatch(label, -1)
		if len(m) != 3 || len(m[0]) < 2 || len(m[1]) < 2 || len(m[2]) < 2 {
			return label
		}
		return m[0][1] + " > " + m[1][1] + " : " + m[2][1]
	},

	"matchOrMatches": func(i int) string {
		if i == 1 {
			return " (1 match)"
		}
		return fmt.Sprintf(" (%d matches)", i)
	},
}

func streamSearchHighlightDiffPreview(diffPreview string, highlights []highlight) string {
	useColordiff, err := strconv.ParseBool(os.Getenv("COLORDIFF"))
	if err != nil {
		useColordiff = true
	}
	if colorDisabled || !useColordiff {
		// Only highlight the matches.
		return applyHighlights(stripMarkdownMarkers(diffPreview), highlights, ansiColors["search-match"], ansiColors["nc"])
	}
	path, err := exec.LookPath("colordiff")
	if err != nil {
		// colordiff not installed; only highlight the matches.
		return applyHighlights(stripMarkdownMarkers(diffPreview), highlights, ansiColors["search-match"], ansiColors["nc"])
	}

	// First highlight the matches, but use a special "end of match" token
	// instead of no color (so that we don'streamingTemplate terminate colors that colordiff
	// adds).
	uniqueStartOfMatchToken := "pXRdMhZbgnPL355429nsO4qFgX86LfXTSmqH4Nr3#*(@)!*#()@!APPJB8ZRutvZ5fdL01273i6OdzLDm0UMC9372891skfJTl2c52yR1v"
	uniqueEndOfMatchToken := "v1Ry25c2lTJfks1982739CMU0mDLzdO6i37210Ldf5ZvtuRZ8BJPPA!@)(#*!)@(*#3rN4HqmSTXfL68XgFq4Osn924553LPngbZhMdRXp"
	diff := applyHighlights(stripMarkdownMarkers(diffPreview), highlights, uniqueStartOfMatchToken, uniqueEndOfMatchToken)

	// Now highlight our diff with colordiff.
	var buf bytes.Buffer
	cmd := exec.Command(path)
	cmd.Stdin = strings.NewReader(diff)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		fmt.Println("warning: colordiff failed to colorize diff:", err)
		return diff
	}
	colorized := buf.String()
	var final []string
	for _, line := range strings.Split(colorized, "\n") {
		// fmt.Println("LINE", line)
		// Find where the start-of-match token is in the line.
		somToken := strings.Index(line, uniqueStartOfMatchToken)

		// Find which ANSI codes are to the left of our start-of-match token.
		indices := ansiRegexp.FindAllStringIndex(line, -1)
		matches := ansiRegexp.FindAllString(line, -1)
		var left []string
		for k, index := range indices {
			if index[0] < somToken && index[1] < somToken {
				left = append(left, matches[k])
			}
		}

		// Replace our start-of-match token with the color we wish.
		line = strings.Replace(line, uniqueStartOfMatchToken, ansiColors["search-match"], -1)

		// Replace our end-of-match token with the color terminator,
		// and start all colors that were previously started to the left.
		line = strings.Replace(line, uniqueEndOfMatchToken, ansiColors["nc"]+strings.Join(left, ""), -1)

		final = append(final, line)
	}
	return strings.Join(final, "\n")
}

func stripMarkdownMarkers(content string) string {
	content = strings.TrimLeft(content, "```COMMIT_EDITMSG\n")
	content = strings.TrimLeft(content, "```diff\n")
	return strings.TrimRight(content, "\n```")
}

// convertMatchToHighlights converts a FileMatch m to a highlight data type.
// When isPreview is true, it is assumed that the result to highlight is only on
// one line, and the offsets are relative to this line. When isPreview is false,
// the lineNumber from the FileMatch data is used, which is relative to the file
// content.
func streamConvertMatchToHighlights(m streaming.EventLineMatch, isPreview bool) (highlights []highlight) {
	var line int
	for _, offsetAndLength := range m.OffsetAndLengths {
		ol := offsetAndLength
		offset := int(ol[0])
		length := int(ol[1])
		if isPreview {
			line = 1
		} else {
			line = int(m.LineNumber)
		}
		highlights = append(highlights, highlight{line: line, character: offset, length: length})
	}
	return highlights
}
