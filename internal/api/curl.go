package api

import (
	"net/http"
	"sort"

	"github.com/kballard/go-shellquote"
)

// CurlCommand renders req (with the given body) as a single-line, shell-safe
// curl invocation. Headers are sorted for deterministic output. The body is
// included verbatim with -d when non-empty.
func CurlCommand(req *http.Request, body []byte) string {
	args := []string{"curl", "-X", req.Method}

	headerNames := make([]string, 0, len(req.Header))
	for name := range req.Header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		values := append([]string{}, req.Header.Values(name)...)
		sort.Strings(values)
		for _, value := range values {
			args = append(args, "-H", name+": "+value)
		}
	}

	if len(body) > 0 {
		args = append(args, "-d", string(body))
	}
	args = append(args, req.URL.String())
	return shellquote.Join(args...)
}
