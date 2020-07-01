package main

import (
	"os"
	"strings"
)

// getAdditionalHeaders reads the environment for values like SRC_HEADER_NAME=VALUE
// and creates a `{NAME: VALUE}` map. These headers should be applied to each request
// to the Sourcegraph instance, as some private instances require special auth or
// additional proxy values to be passed along with each request.
func parseAdditionalHeaders() map[string]string {
	return parseAdditionalHeadersFromMap(os.Environ())
}

const additionalHeaderPrefix = "SRC_HEADER_"

func parseAdditionalHeadersFromMap(environ []string) map[string]string {
	additionalHeaders := map[string]string{}
	for _, value := range environ {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 || len(parts[0]) <= len(additionalHeaderPrefix) || len(parts[1]) == 0 || !strings.HasPrefix(parts[0], additionalHeaderPrefix) {
			continue
		}

		additionalHeaders[strings.TrimPrefix(parts[0], additionalHeaderPrefix)] = parts[1]
	}

	return additionalHeaders
}
