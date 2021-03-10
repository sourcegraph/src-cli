package streaming

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/sourcegraph/src-cli/internal/api"
)

// Opts contains the search options supported by Search.
type Opts struct {
	Display int
	Trace   bool
	Json    bool
}

// Search calls the streaming search endpoint and uses decoder to decode the
// response body.
func Search(query string, opts Opts, client api.Client, decoder Decoder) error {
	// Create request.
	req, err := client.NewHTTPRequest(context.Background(), "GET", "search/stream?q="+url.QueryEscape(query), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if opts.Display >= 0 {
		q := req.URL.Query()
		q.Add("display", strconv.Itoa(opts.Display))
		req.URL.RawQuery = q.Encode()
	}

	// Send request.
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Process response.
	err = decoder.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error during decoding: %w", err)
	}

	// Output trace.
	if opts.Trace {
		_, err = fmt.Fprintf(os.Stderr, fmt.Sprintf("\nx-trace: %s\n", resp.Header.Get("x-trace")))
		if err != nil {
			return err
		}
	}
	return nil
}
