package main

import (
	"os"
	"regexp"
	"strings"
)

// parseAdditionalHeaders reads the environment for values like SRC_HEADER_NAME=VALUE or
// SRC_HEADERS and creates a `{NAME: VALUE}` map. These headers should be applied to each
// request to the Sourcegraph instance, as some private instances require special auth or
// additional proxy values to be passed along with each request.
func parseAdditionalHeaders() map[string]string {
	return parseAdditionalHeadersFromEnviron(os.Environ())
}

const additionalHeaderPrefix = "SRC_HEADER_"
const additionalHeadersKey = "SRC_HEADERS"

func parseAdditionalHeadersFromEnviron(environ []string) map[string]string {
	additionalHeaders := map[string]string{}
	for _, value := range environ {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			continue
		}

		if parts[0] == additionalHeadersKey && parts[1] != "" {
			// This regex removes all leading and trailing spaces from the environment variable. We need to do this
			// because most shells add quotes to a string when it contains a new line. Tested with `bash, fish & zsh`
			// and they all have the same behavior.
			re := regexp.MustCompile(`^"|"$`)
			headers := re.ReplaceAllString(parts[1], "")
			headers = strings.Replace(headers, `\n`, "\n", -1)
			splitHeaders := strings.Split(headers, "\n")

			for _, h := range splitHeaders {
				p := strings.SplitN(h, ":", 2)
				if len(parts) != 2 || p[1] == "" {
					continue
				}

				additionalHeaders[strings.ToLower(p[0])] = strings.Trim(p[1], " ")
			}
			continue
		}

		// Ensure we'll have a non-empty key after trimming
		if !strings.HasPrefix(parts[0], additionalHeaderPrefix) || len(parts[0]) <= len(additionalHeaderPrefix) {
			continue
		}

		// Ensure we have a non-empty value
		if parts[1] == "" {
			continue
		}

		additionalHeaders[strings.ToLower(strings.TrimPrefix(parts[0], additionalHeaderPrefix))] = parts[1]
	}

	return additionalHeaders
}
