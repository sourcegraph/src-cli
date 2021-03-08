package batches

import (
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

// UnsupportedRepoSet provides a set to manage repositories that are on
// unsupported code hosts. This type implements error to allow it to be
// returned directly as an error value if needed.
type UnsupportedRepoSet map[*graphql.Repository]struct{}

func (e UnsupportedRepoSet) includes(r *graphql.Repository) bool {
	_, ok := e[r]
	return ok
}

func (e UnsupportedRepoSet) Error() string {
	repos := []string{}
	typeSet := map[string]struct{}{}
	for repo := range e {
		repos = append(repos, repo.Name)
		typeSet[repo.ExternalRepository.ServiceType] = struct{}{}
	}

	types := []string{}
	for t := range typeSet {
		types = append(types, t)
	}

	return fmt.Sprintf(
		"found repositories on unsupported code hosts: %s\nrepositories:\n\t%s",
		strings.Join(types, ", "),
		strings.Join(repos, "\n\t"),
	)
}

func (e UnsupportedRepoSet) appendRepo(repo *graphql.Repository) {
	e[repo] = struct{}{}
}

func (e UnsupportedRepoSet) hasUnsupported() bool {
	return len(e) > 0
}
