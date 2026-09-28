package docker

import (
	"fmt"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// BindMount returns a Docker bind mount specification after checking that its
// values cannot inject fields into Docker's comma-delimited mount grammar.
func BindMount(source, target string, readOnly bool) (string, error) {
	for name, value := range map[string]string{"source": source, "target": target} {
		if value == "" || strings.ContainsAny(value, ",\r\n\x00") {
			return "", errors.Newf("invalid Docker mount %s %q", name, value)
		}
	}

	mount := fmt.Sprintf("type=bind,source=%s,target=%s", source, target)
	if readOnly {
		mount += ",ro"
	}
	return mount, nil
}
